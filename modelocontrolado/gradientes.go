package modelocontrolado

import (
	"fmt"
	"math"
)

// GradienteParcial representa el resultado del cálculo
// de gradientes sobre un mini-batch local.
//
// El gradiente se conserva como suma ponderada.
// La normalización se realiza después de agregar
// los resultados de todos los nodos.
type GradienteParcial struct {
	Gradiente []float64 `json:"gradiente"`
	SumaPesos float64   `json:"suma_pesos"`
	Muestras  int       `json:"muestras"`
	Positivos int       `json:"positivos"`
}

// calcularGradienteLocal procesa un conjunto de ejemplos
// utilizando los pesos globales actuales.
func (m *Modelo) calcularGradienteLocal(
	datos []Ejemplo,
) (GradienteParcial, error) {

	resultado := GradienteParcial{
		Gradiente: make([]float64, DimensionModelo),
	}

	if m == nil || len(m.Pesos) != DimensionModelo {
		return resultado, fmt.Errorf("modelo no inicializado")
	}

	if len(datos) == 0 {
		return resultado, fmt.Errorf("mini-batch vacío")
	}

	if m.PesoPositivo <= 0 ||
		math.IsNaN(m.PesoPositivo) ||
		math.IsInf(m.PesoPositivo, 0) {
		return resultado, fmt.Errorf("peso positivo inválido")
	}

	for i := range datos {
		e := &datos[i]

		if err := validarEjemplo(*e); err != nil {
			return resultado, fmt.Errorf(
				"ejemplo %d inválido: %w", i, err,
			)
		}

		if e.Clase > 1 {
			return resultado, fmt.Errorf(
				"clase inválida en ejemplo %d", i,
			)
		}

		// Ponderación de la clase positiva.
		peso := 1.0

		if e.Clase == 1 {
			peso = m.PesoPositivo
			resultado.Positivos++
		}

		// Error de la predicción logística.
		delta := (sigmoid(m.puntaje(e)) - float64(e.Clase)) * peso
		// Gradiente del intercepto.
		resultado.Gradiente[0] += delta

		// Gradiente del mes.
		resultado.Gradiente[offMes+int(e.Mes)] += delta

		// Variables categóricas.
		resultado.Gradiente[offMetodo+int(e.Metodo)] += delta

		resultado.Gradiente[offCategoria+int(e.Categoria)] += delta

		resultado.Gradiente[offComprador+int(e.Comprador)] += delta

		resultado.Gradiente[offDetalle+int(e.Detalle)] += delta

		resultado.Gradiente[offCombinacion+int(e.Combinacion)] += delta

		// Gradiente de los tokens del título.
		if e.NTokens > 0 {
			escala := delta /
				math.Sqrt(float64(e.NTokens))

			for j := uint8(0); j < e.NTokens; j++ {
				indice := offTexto + int(e.Tokens[j])
				resultado.Gradiente[indice] += escala
			}
		}

		// Gradiente de los tokens de la descripción.
		if m.UsaDescripcion && e.NTokensDesc > 0 {
			escala := delta /
				math.Sqrt(float64(e.NTokensDesc))

			for j := uint8(0); j < e.NTokensDesc; j++ {
				indice := offDescripcion +
					int(e.TokensDesc[j])

				resultado.Gradiente[indice] += escala
			}
		}

		resultado.SumaPesos += peso
		resultado.Muestras++
	}

	return resultado, nil
}

// CalcularGradiente calcula el gradiente de un mini-batch
// utilizando una única goroutine.
func (m *Modelo) CalcularGradiente(
	datos []Ejemplo,
) (GradienteParcial, error) {

	return m.calcularGradienteLocal(datos)
}

// CalcularGradienteParalelo divide el mini-batch entre
// varias goroutines y combina sus gradientes parciales.
func (m *Modelo) CalcularGradienteParalelo(
	datos []Ejemplo,
	workers int,
) (GradienteParcial, error) {

	resultado := GradienteParcial{
		Gradiente: make([]float64, DimensionModelo),
	}

	if workers < 1 {
		return resultado, fmt.Errorf(
			"el número de workers debe ser mayor que cero",
		)
	}

	if len(datos) == 0 {
		return resultado, fmt.Errorf("mini-batch vacío")
	}

	if workers > len(datos) {
		workers = len(datos)
	}

	type respuesta struct {
		parcial GradienteParcial
		err     error
	}

	ch := make(chan respuesta, workers)

	// Dividir los registros entre las goroutines.
	for k := 0; k < workers; k++ {
		inicio := k * len(datos) / workers
		fin := (k + 1) * len(datos) / workers

		go func(subconjunto []Ejemplo) {
			parcial, err :=
				m.calcularGradienteLocal(subconjunto)

			ch <- respuesta{
				parcial: parcial,
				err:     err,
			}
		}(datos[inicio:fin])
	}

	// Reunir los resultados parciales.
	var primerError error

	for k := 0; k < workers; k++ {
		res := <-ch

		if res.err != nil {
			if primerError == nil {
				primerError = res.err
			}
			continue
		}

		resultado.SumaPesos += res.parcial.SumaPesos
		resultado.Muestras += res.parcial.Muestras
		resultado.Positivos += res.parcial.Positivos

		for j := 0; j < DimensionModelo; j++ {
			resultado.Gradiente[j] +=
				res.parcial.Gradiente[j]
		}
	}

	if primerError != nil {
		return GradienteParcial{}, primerError
	}

	return resultado, nil
}
