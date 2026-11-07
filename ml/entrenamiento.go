package ml

import (
	"errors"
	"sync"
	"time"
)

// Config define los hiperparámetros del entrenamiento paralelo.
type Config struct {
	Workers int     `json:"workers"`  // goroutines que calculan gradientes
	Epocas  int     `json:"epocas"`   // pasadas completas sobre los datos
	TamLote int     `json:"tam_lote"` // filas por actualización (global); 0 = lote completo
	Tasa    float64 `json:"tasa"`     // tasa de aprendizaje
	Momento float64 `json:"momento"`  // momento (0 = descenso de gradiente simple)
	L2      float64 `json:"l2"`       // regularización L2 (no se aplica al sesgo)

	// AlFinalizarEpoca, si no es nil, recibe la pérdida media de cada época.
	AlFinalizarEpoca func(epoca int, perdida float64) `json:"-"`
}

// Resultado resume un entrenamiento.
type Resultado struct {
	Modelo   *Modelo
	Perdidas []float64 // pérdida media ponderada por época
	Pasos    int       // actualizaciones de pesos realizadas
	Duracion time.Duration
}

// tarea es lo que el coordinador envía a un worker en cada paso:
// una instantánea inmutable de los pesos y el rango de filas a procesar.
type tarea struct {
	modelo *Modelo
	desde  int
	hasta  int
}

type respuesta struct {
	worker  int
	parcial Parcial
}

// EntrenarParalelo entrena la regresión logística con paralelismo de datos
// y sincronización por pasos (patrón parameter server / all-reduce):
//
//  1. Los datos se dividen en tantos fragmentos (shards) como workers.
//     Cada worker es una goroutine dueña de su fragmento.
//  2. En cada paso el coordinador envía a cada worker, por su propio canal,
//     una instantánea de los pesos y el rango de su mini-lote.
//  3. Cada worker calcula su gradiente parcial (map) y lo devuelve por un
//     canal de resultados compartido.
//  4. El coordinador espera a todos (barrera), suma los parciales en orden
//     fijo de worker (reduce) y actualiza los pesos.
//
// Solo el coordinador escribe los pesos, y los workers solo leen copias
// que nadie modifica después de enviarlas: no hay condiciones de carrera.
// Sumar en orden fijo hace que el resultado sea reproducible sin importar
// en qué orden terminen las goroutines.
func EntrenarParalelo(d *Datos, cfg Config) (*Resultado, error) {
	if err := validarConfig(d, cfg); err != nil {
		return nil, err
	}

	inicio := time.Now()

	workers := min(cfg.Workers, d.N)
	pesoClase := PesosDeClase(d)

	// Fragmento de cada worker y tamaño del mini-lote por worker.
	desdeFragmento := make([]int, workers)
	hastaFragmento := make([]int, workers)
	mayorFragmento := 0
	for w := range workers {
		desdeFragmento[w], hastaFragmento[w] = rangoFragmento(d.N, workers, w)
		mayorFragmento = max(mayorFragmento, hastaFragmento[w]-desdeFragmento[w])
	}

	filasPorWorker := mayorFragmento
	if cfg.TamLote > 0 {
		filasPorWorker = max(1, (cfg.TamLote+workers-1)/workers)
	}
	pasosPorEpoca := (mayorFragmento + filasPorWorker - 1) / filasPorWorker

	// Pool de workers: un canal de tareas por worker y un canal de
	// respuestas compartido con buffer para que nadie se bloquee al responder.
	tareas := make([]chan tarea, workers)
	respuestas := make(chan respuesta, workers)
	var wg sync.WaitGroup

	for w := range workers {
		tareas[w] = make(chan tarea, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tareas[w] {
				respuestas <- respuesta{
					worker:  w,
					parcial: CalcularParcial(t.modelo, d, t.desde, t.hasta, pesoClase),
				}
			}
		}()
	}

	defer func() {
		for _, canal := range tareas {
			close(canal)
		}
		wg.Wait()
	}()

	modelo := NuevoModelo(d.P)
	velocidad := make([]float64, d.P+1)
	gradiente := make([]float64, d.P+1)
	parciales := make([]Parcial, workers)
	resultado := &Resultado{}

	for epoca := 1; epoca <= cfg.Epocas; epoca++ {
		var perdidaEpoca, pesoEpoca float64

		for paso := range pasosPorEpoca {
			instantanea := modelo.Clonar()

			// Difusión (broadcast) de la tarea a cada worker con filas pendientes.
			activos := 0
			for w := range workers {
				desde := desdeFragmento[w] + paso*filasPorWorker
				hasta := min(desde+filasPorWorker, hastaFragmento[w])
				if desde >= hasta {
					continue
				}
				tareas[w] <- tarea{modelo: instantanea, desde: desde, hasta: hasta}
				activos++
			}

			// Barrera: se esperan todas las respuestas del paso.
			for range activos {
				r := <-respuestas
				parciales[r.worker] = r.parcial
			}

			// Reducción en orden fijo de worker.
			for j := range gradiente {
				gradiente[j] = 0
			}
			pesoPaso := 0.0
			for w := range parciales {
				p := parciales[w]
				if p.Peso == 0 {
					continue
				}
				for j, g := range p.Gradiente {
					gradiente[j] += g
				}
				pesoPaso += p.Peso
				perdidaEpoca += p.Perdida
				parciales[w] = Parcial{}
			}

			if pesoPaso == 0 {
				continue
			}
			pesoEpoca += pesoPaso

			actualizarPesos(modelo, velocidad, gradiente, pesoPaso, cfg)
			resultado.Pasos++
		}

		perdidaMedia := perdidaEpoca / pesoEpoca
		resultado.Perdidas = append(resultado.Perdidas, perdidaMedia)
		if cfg.AlFinalizarEpoca != nil {
			cfg.AlFinalizarEpoca(epoca, perdidaMedia)
		}
	}

	resultado.Modelo = modelo
	resultado.Duracion = time.Since(inicio)
	return resultado, nil
}

// actualizarPesos aplica descenso de gradiente con momento sobre el
// gradiente promedio del paso, más la penalización L2 en los pesos.
func actualizarPesos(m *Modelo, velocidad, gradiente []float64, peso float64, cfg Config) {
	p := len(m.Pesos)

	for j := range p {
		g := gradiente[j]/peso + cfg.L2*m.Pesos[j]
		velocidad[j] = cfg.Momento*velocidad[j] + g
		m.Pesos[j] -= cfg.Tasa * velocidad[j]
	}

	g := gradiente[p] / peso
	velocidad[p] = cfg.Momento*velocidad[p] + g
	m.Sesgo -= cfg.Tasa * velocidad[p]
}

func validarConfig(d *Datos, cfg Config) error {
	switch {
	case d == nil || d.N == 0:
		return errors.New("no hay datos de entrenamiento")
	case cfg.Workers < 1:
		return errors.New("debe haber al menos un worker")
	case cfg.Epocas < 1:
		return errors.New("debe haber al menos una época")
	case cfg.TamLote < 0:
		return errors.New("el tamaño de lote no puede ser negativo")
	case cfg.Tasa <= 0:
		return errors.New("la tasa de aprendizaje debe ser positiva")
	case cfg.Momento < 0 || cfg.Momento >= 1:
		return errors.New("el momento debe estar en [0, 1)")
	case cfg.L2 < 0:
		return errors.New("la regularización L2 no puede ser negativa")
	}
	return nil
}
