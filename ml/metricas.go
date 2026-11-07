package ml

import (
	"encoding/json"
	"os"
	"sort"
	"time"
)

// Metricas resume el desempeño del modelo sobre un conjunto de datos.
type Metricas struct {
	Umbral       float64 `json:"umbral"`
	Exactitud    float64 `json:"exactitud"`
	Precision    float64 `json:"precision"`
	Sensibilidad float64 `json:"sensibilidad"` // recall de la clase 1
	F1           float64 `json:"f1"`
	AUC          float64 `json:"auc"`
	PerdidaLog   float64 `json:"perdida_log"` // entropía cruzada sin ponderar
	VP           int     `json:"vp"`
	FP           int     `json:"fp"`
	VN           int     `json:"vn"`
	FN           int     `json:"fn"`
}

// Evaluar calcula las métricas de clasificación con el umbral dado.
// Si los datos tienen una sola clase, el AUC se reporta como 0.5.
func Evaluar(m *Modelo, d *Datos, umbral float64) Metricas {
	met := Metricas{Umbral: umbral}
	puntajes := make([]float64, d.N)
	perdida := 0.0

	for i := range d.N {
		z := m.Logit(d.Fila(i))
		puntajes[i] = sigmoide(z)
		perdida += perdidaLogistica(z, d.Y[i])

		prediccion := puntajes[i] >= umbral
		real := d.Y[i] == 1
		switch {
		case prediccion && real:
			met.VP++
		case prediccion && !real:
			met.FP++
		case !prediccion && !real:
			met.VN++
		default:
			met.FN++
		}
	}

	if d.N > 0 {
		met.Exactitud = float64(met.VP+met.VN) / float64(d.N)
		met.PerdidaLog = perdida / float64(d.N)
	}
	met.Precision = dividir(met.VP, met.VP+met.FP)
	met.Sensibilidad = dividir(met.VP, met.VP+met.FN)
	if met.Precision+met.Sensibilidad > 0 {
		met.F1 = 2 * met.Precision * met.Sensibilidad / (met.Precision + met.Sensibilidad)
	}
	met.AUC = calcularAUC(puntajes, d.Y)

	return met
}

func dividir(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// calcularAUC usa la equivalencia entre el área bajo la curva ROC y el
// estadístico de Mann-Whitney: ordena los puntajes y suma los rangos de los
// positivos (con rango promedio en los empates).
func calcularAUC(puntajes, y []float64) float64 {
	orden := make([]int, len(puntajes))
	for i := range orden {
		orden[i] = i
	}
	sort.Slice(orden, func(a, b int) bool {
		return puntajes[orden[a]] < puntajes[orden[b]]
	})

	var positivos, negativos, sumaRangos float64
	for i := 0; i < len(orden); {
		j := i
		for j < len(orden) && puntajes[orden[j]] == puntajes[orden[i]] {
			j++
		}
		rangoPromedio := float64(i+j+1) / 2 // rangos i+1 .. j
		for k := i; k < j; k++ {
			if y[orden[k]] == 1 {
				positivos++
				sumaRangos += rangoPromedio
			} else {
				negativos++
			}
		}
		i = j
	}

	if positivos == 0 || negativos == 0 {
		return 0.5
	}
	return (sumaRangos - positivos*(positivos+1)/2) / (positivos * negativos)
}

// ArchivoModelo es lo que se guarda en disco tras entrenar: los pesos,
// el codificador para transformar nuevas consultas y la evidencia de
// cómo se entrenó. La API de predicciones lo cargará en la PC4.
type ArchivoModelo struct {
	Modelo          *Modelo      `json:"modelo"`
	Codificador     *Codificador `json:"codificador"`
	Caracteristicas []string     `json:"caracteristicas"`
	Config          Config       `json:"config"`
	MetricasPrueba  Metricas     `json:"metricas_prueba"`
	Entrenado       time.Time    `json:"entrenado"`
}

// Predecir devuelve la probabilidad de riesgo de un registro nuevo.
func (a *ArchivoModelo) Predecir(r Registro) float64 {
	x := make([]float64, a.Codificador.NumCaracteristicas())
	a.Codificador.Codificar(r, x)
	return a.Modelo.Probabilidad(x)
}

// GuardarArchivoModelo escribe el modelo en formato JSON.
func GuardarArchivoModelo(ruta string, a *ArchivoModelo) error {
	contenido, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ruta, contenido, 0o644)
}

// CargarArchivoModelo lee un modelo guardado con GuardarArchivoModelo.
func CargarArchivoModelo(ruta string) (*ArchivoModelo, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, err
	}
	var a ArchivoModelo
	if err := json.Unmarshal(contenido, &a); err != nil {
		return nil, err
	}
	return &a, nil
}
