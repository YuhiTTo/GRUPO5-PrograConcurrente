package ml

import "math"

// Modelo es una regresión logística: P(riesgo=1 | x) = sigmoide(w·x + b).
type Modelo struct {
	Pesos []float64 `json:"pesos"`
	Sesgo float64   `json:"sesgo"`
}

// NuevoModelo crea un modelo con p pesos en cero.
func NuevoModelo(p int) *Modelo {
	return &Modelo{Pesos: make([]float64, p)}
}

// Clonar devuelve una copia independiente del modelo.
func (m *Modelo) Clonar() *Modelo {
	pesos := make([]float64, len(m.Pesos))
	copy(pesos, m.Pesos)
	return &Modelo{Pesos: pesos, Sesgo: m.Sesgo}
}

// Logit calcula w·x + b.
func (m *Modelo) Logit(x []float64) float64 {
	z := m.Sesgo
	for j, w := range m.Pesos {
		z += w * x[j]
	}
	return z
}

// Probabilidad devuelve la probabilidad estimada de riesgo (clase 1).
func (m *Modelo) Probabilidad(x []float64) float64 {
	return sigmoide(m.Logit(x))
}

// sigmoide evita el desbordamiento de exp para valores grandes de |z|.
func sigmoide(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}

// perdidaLogistica es la entropía cruzada binaria escrita en función del
// logit z, estable aun cuando la probabilidad está muy cerca de 0 o 1.
func perdidaLogistica(z, y float64) float64 {
	return math.Max(z, 0) - y*z + math.Log1p(math.Exp(-math.Abs(z)))
}

// Parcial es el resultado que un worker calcula sobre su fragmento:
// la suma (no el promedio) de gradientes y pérdidas ponderadas.
// Usar sumas permite combinar parciales de fragmentos de distinto tamaño
// simplemente sumándolos.
type Parcial struct {
	Gradiente []float64 // longitud P+1; la última posición es el sesgo
	Perdida   float64   // suma de pérdidas ponderadas
	Peso      float64   // suma de pesos de clase de las filas procesadas
}

// CalcularParcial es el paso "map" del entrenamiento: recorre las filas
// [desde, hasta) y acumula el gradiente de la entropía cruzada ponderada.
// Solo lee el modelo y los datos; no modifica nada compartido.
func CalcularParcial(m *Modelo, d *Datos, desde, hasta int, pesoClase [2]float64) Parcial {
	parcial := Parcial{Gradiente: make([]float64, d.P+1)}
	gradiente := parcial.Gradiente

	for i := desde; i < hasta; i++ {
		x := d.Fila(i)
		y := d.Y[i]
		peso := pesoClase[int(y)]

		z := m.Logit(x)
		diferencia := peso * (sigmoide(z) - y)

		for j, xj := range x {
			gradiente[j] += diferencia * xj
		}
		gradiente[d.P] += diferencia

		parcial.Perdida += peso * perdidaLogistica(z, y)
		parcial.Peso += peso
	}

	return parcial
}
