package datos

import (
	"fmt"
	"math/rand"

	"grupo5-prograconcurrente/modelocontrolado"
)

const SemillaEntrenamiento int64 = 42

// BarajarEpoca reproduce el barajado determinista
// utilizado en el entrenamiento de PC3.
func BarajarEpoca(
	ejemplos []modelocontrolado.Ejemplo,
	rng *rand.Rand,
) error {

	if rng == nil {
		return fmt.Errorf("generador aleatorio no inicializado")
	}

	rng.Shuffle(len(ejemplos), func(i, j int) {
		ejemplos[i], ejemplos[j] =
			ejemplos[j], ejemplos[i]
	})

	return nil
}

// RangoWorker divide un mini-lote entre workers
// mediante segmentos contiguos sin solapamientos.
func RangoWorker(
	inicio int,
	fin int,
	workerID int,
	totalWorkers int,
) (int, int, error) {

	if inicio < 0 || fin <= inicio {
		return 0, 0, fmt.Errorf("rango de mini-lote inválido")
	}

	if totalWorkers < 1 {
		return 0, 0, fmt.Errorf("cantidad de workers inválida")
	}

	if workerID < 0 || workerID >= totalWorkers {
		return 0, 0, fmt.Errorf("identificador de worker inválido")
	}

	total := fin - inicio

	desde := inicio + workerID*total/totalWorkers
	hasta := inicio + (workerID+1)*total/totalWorkers

	return desde, hasta, nil
}
