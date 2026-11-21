package modelocontrolado

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

// Modelo almacena los parámetros aprendidos en la PC3.
type Modelo struct {
	Descripcion string `json:"descripcion"`

	AnioInicioEntrenamiento int `json:"anio_inicio_entrenamiento"`
	AnioFinEntrenamiento    int `json:"anio_fin_entrenamiento"`
	AnioValidacion          int `json:"anio_validacion"`

	Workers                int     `json:"workers"`
	MejorEpoca             int     `json:"mejor_epoca"`
	PesoPositivo           float64 `json:"peso_positivo"`
	TasaAdam               float64 `json:"tasa_adam"`
	Umbral                 float64 `json:"umbral_seleccionado_en_validacion"`
	RegistrosEntrenamiento int     `json:"registros_entrenamiento"`
	RegistrosValidacion    int     `json:"registros_validacion"`

	UsaDescripcion bool      `json:"usa_descripcion"`
	Pesos          []float64 `json:"pesos"`
}

// Prediccion representa el resultado de una evaluación.
type Prediccion struct {
	Probabilidad  float64 `json:"probabilidad"`
	ClasePredicha uint8   `json:"clase_predicha"`
	Umbral        float64 `json:"umbral"`
}

// CargarModelo carga los pesos aprendidos desde un archivo JSON.
func CargarModelo(ruta string) (*Modelo, error) {
	contenido, err := os.ReadFile(ruta)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer el modelo: %w", err)
	}

	var modelo Modelo

	if err := json.Unmarshal(contenido, &modelo); err != nil {
		return nil, fmt.Errorf("JSON del modelo inválido: %w", err)
	}

	if len(modelo.Pesos) != DimensionModelo {
		return nil, fmt.Errorf(
			"dimensiones incorrectas: esperado %d, recibido %d",
			DimensionModelo,
			len(modelo.Pesos),
		)
	}

	if math.IsNaN(modelo.Umbral) ||
		math.IsInf(modelo.Umbral, 0) ||
		modelo.Umbral < 0 ||
		modelo.Umbral > 1 {
		return nil, fmt.Errorf("umbral de clasificación inválido")
	}

	for i, peso := range modelo.Pesos {
		if math.IsNaN(peso) || math.IsInf(peso, 0) {
			return nil, fmt.Errorf("peso inválido en posición %d", i)
		}
	}

	return &modelo, nil
}

// sigmoid convierte el puntaje lineal en una probabilidad.
func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}

	x := math.Exp(z)
	return x / (1 + x)
}

// validarEjemplo comprueba que las características estén dentro
// de los límites definidos por el modelo.
func validarEjemplo(e Ejemplo) error {
	if e.Mes >= nMes {
		return fmt.Errorf("mes fuera de rango")
	}

	if int(e.Metodo) >= nMetodo ||
		int(e.Categoria) >= nCategoria ||
		int(e.Comprador) >= nComprador ||
		int(e.Detalle) >= nDetalle ||
		int(e.Combinacion) >= nCombinacion {
		return fmt.Errorf("índice de característica fuera de rango")
	}

	if int(e.NTokens) > maxTokens ||
		int(e.NTokensDesc) > maxTokensDesc {
		return fmt.Errorf("cantidad de tokens inválida")
	}

	for i := 0; i < int(e.NTokens); i++ {
		if int(e.Tokens[i]) >= nTexto {
			return fmt.Errorf("token del título fuera de rango")
		}
	}

	for i := 0; i < int(e.NTokensDesc); i++ {
		if int(e.TokensDesc[i]) >= nDescripcion {
			return fmt.Errorf("token de descripción fuera de rango")
		}
	}

	return nil
}

// puntaje reproduce la función de puntuación utilizada en PC3.
func (m *Modelo) puntaje(e *Ejemplo) float64 {
	w := m.Pesos

	z := w[0] + w[offMes+int(e.Mes)]

	z += w[offMetodo+int(e.Metodo)]
	z += w[offCategoria+int(e.Categoria)]
	z += w[offComprador+int(e.Comprador)]
	z += w[offDetalle+int(e.Detalle)]
	z += w[offCombinacion+int(e.Combinacion)]

	if e.NTokens > 0 {
		escala := 1 / math.Sqrt(float64(e.NTokens))

		for i := uint8(0); i < e.NTokens; i++ {
			z += escala * w[offTexto+int(e.Tokens[i])]
		}
	}

	if m.UsaDescripcion && e.NTokensDesc > 0 {
		escala := 1 / math.Sqrt(float64(e.NTokensDesc))

		for i := uint8(0); i < e.NTokensDesc; i++ {
			z += escala * w[offDescripcion+int(e.TokensDesc[i])]
		}
	}

	return z
}

// PredecirEjemplo calcula la probabilidad y la clase predicha
// a partir de un ejemplo previamente codificado.
func (m *Modelo) PredecirEjemplo(e Ejemplo) (Prediccion, error) {
	var resultado Prediccion

	if m == nil || len(m.Pesos) != DimensionModelo {
		return resultado, fmt.Errorf("modelo no inicializado")
	}

	if err := validarEjemplo(e); err != nil {
		return resultado, err
	}

	probabilidad := sigmoid(m.puntaje(&e))

	clase := uint8(0)
	if probabilidad >= m.Umbral {
		clase = 1
	}

	resultado = Prediccion{
		Probabilidad:  probabilidad,
		ClasePredicha: clase,
		Umbral:        m.Umbral,
	}

	return resultado, nil
}

// Predecir transforma una contratación y calcula su resultado.
func (m *Modelo) Predecir(r Registro) (Prediccion, error) {
	var resultado Prediccion

	if m == nil {
		return resultado, fmt.Errorf("modelo no inicializado")
	}

	ejemplo, err := Codificar(r, m.UsaDescripcion)
	if err != nil {
		return resultado, fmt.Errorf("error al codificar contratación: %w", err)
	}

	return m.PredecirEjemplo(ejemplo)
}
