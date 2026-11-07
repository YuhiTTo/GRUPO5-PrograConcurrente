package ml

import (
	"math/rand/v2"
	"sync"
)

// Datos es la matriz de características en memoria contigua (fila por fila)
// junto con sus etiquetas. Una vez construida es de solo lectura, por eso
// varias goroutines pueden leerla a la vez sin sincronización.
type Datos struct {
	N int       // número de filas
	P int       // número de características
	X []float64 // N*P valores, fila i en X[i*P:(i+1)*P]
	Y []float64 // N etiquetas (0 o 1)
}

// Fila devuelve las características de la fila i sin copiarlas.
func (d *Datos) Fila(i int) []float64 {
	return d.X[i*d.P : (i+1)*d.P]
}

// ConstruirDatos codifica los registros usando varias goroutines.
// Cada goroutine escribe un rango de filas disjunto de la misma matriz,
// así que no hay condición de carrera ni necesidad de mutex.
func ConstruirDatos(registros []Registro, c *Codificador, workers int) *Datos {
	p := c.NumCaracteristicas()
	d := &Datos{
		N: len(registros),
		P: p,
		X: make([]float64, len(registros)*p),
		Y: make([]float64, len(registros)),
	}

	if workers < 1 {
		workers = 1
	}
	if workers > d.N && d.N > 0 {
		workers = d.N
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		desde, hasta := rangoFragmento(d.N, workers, w)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := desde; i < hasta; i++ {
				c.Codificar(registros[i], d.Fila(i))
				d.Y[i] = registros[i].Riesgo
			}
		}()
	}
	wg.Wait()

	return d
}

// rangoFragmento divide n filas en partes casi iguales y devuelve
// el rango [desde, hasta) de la parte w.
func rangoFragmento(n, partes, w int) (int, int) {
	return w * n / partes, (w + 1) * n / partes
}

// Barajar mezcla los registros con una semilla fija para que
// cada ejecución use la misma partición entrenamiento/prueba.
func Barajar(registros []Registro, semilla uint64) {
	aleatorio := rand.New(rand.NewPCG(semilla, semilla^0x9e3779b97f4a7c15))
	aleatorio.Shuffle(len(registros), func(i, j int) {
		registros[i], registros[j] = registros[j], registros[i]
	})
}

// Dividir separa los registros en entrenamiento y prueba.
// Se espera que antes se hayan barajado.
func Dividir(registros []Registro, fraccionPrueba float64) (entrenamiento, prueba []Registro) {
	nPrueba := int(float64(len(registros)) * fraccionPrueba)
	corte := len(registros) - nPrueba
	return registros[:corte], registros[corte:]
}

// PesosDeClase calcula pesos inversamente proporcionales a la frecuencia
// de cada clase (n / (2 * n_clase)), para que la clase minoritaria
// (procedimientos fallidos) no quede ignorada por el modelo.
func PesosDeClase(d *Datos) [2]float64 {
	var conteo [2]float64
	for _, y := range d.Y {
		conteo[int(y)]++
	}

	pesos := [2]float64{1, 1}
	for c := range conteo {
		if conteo[c] > 0 {
			pesos[c] = float64(d.N) / (2 * conteo[c])
		}
	}
	return pesos
}
