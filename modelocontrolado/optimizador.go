package modelocontrolado

import (
	"fmt"
	"math"
)

// OptimizadorAdamW conserva los estados del optimizador
// entre las diferentes iteraciones de entrenamiento.
type OptimizadorAdamW struct {
	M         []float64
	V         []float64
	Iteracion int
	Tasa      float64
}

const (
	beta1       = 0.9
	beta2       = 0.999
	epsilon     = 1e-8
	weightDecay = 0.0005
)

// NuevoOptimizadorAdamW inicializa el optimizador.
func NuevoOptimizadorAdamW(tasa float64) (*OptimizadorAdamW, error) {

	if tasa <= 0 || tasa > 1 ||
		math.IsNaN(tasa) || math.IsInf(tasa, 0) {
		return nil, fmt.Errorf("tasa de aprendizaje inválida")
	}

	return &OptimizadorAdamW{
		M:    make([]float64, DimensionModelo),
		V:    make([]float64, DimensionModelo),
		Tasa: tasa,
	}, nil
}

// Actualizar combina el gradiente recibido y actualiza
// los pesos globales utilizando AdamW.
func (o *OptimizadorAdamW) Actualizar(
	modelo *Modelo,
	grad GradienteParcial,
) error {

	if o == nil || modelo == nil {
		return fmt.Errorf("optimizador o modelo no inicializado")
	}

	if len(modelo.Pesos) != DimensionModelo ||
		len(o.M) != DimensionModelo ||
		len(o.V) != DimensionModelo {
		return fmt.Errorf("dimensiones incompatibles")
	}

	if len(grad.Gradiente) != DimensionModelo {
		return fmt.Errorf("dimensiones del gradiente incorrectas")
	}

	if grad.Muestras <= 0 ||
		grad.SumaPesos <= 0 ||
		math.IsNaN(grad.SumaPesos) ||
		math.IsInf(grad.SumaPesos, 0) {
		return fmt.Errorf("gradiente global inválido")
	}

	// Verificar que todos los gradientes sean válidos.
	for i, valor := range grad.Gradiente {
		if math.IsNaN(valor) || math.IsInf(valor, 0) {
			return fmt.Errorf(
				"gradiente inválido en posición %d", i,
			)
		}
	}

	// Incrementar iteración global.
	o.Iteracion++

	iter := float64(o.Iteracion)

	// Corrección de sesgo.
	b1Corr := 1 - math.Pow(beta1, iter)
	b2Corr := 1 - math.Pow(beta2, iter)

	// Actualizar cada parámetro.
	for j := 0; j < DimensionModelo; j++ {

		// Promedio del gradiente ponderado.
		g := grad.Gradiente[j] / grad.SumaPesos

		// Primer momento.
		o.M[j] = beta1*o.M[j] +
			(1-beta1)*g

		// Segundo momento.
		o.V[j] = beta2*o.V[j] +
			(1-beta2)*g*g

		// Corrección de sesgo.
		mHat := o.M[j] / b1Corr
		vHat := o.V[j] / b2Corr

		// Actualización Adam.
		step := o.Tasa * mHat /
			(math.Sqrt(vHat) + epsilon)

		modelo.Pesos[j] -= step

		// Regularización desacoplada AdamW.
		// El intercepto (posición 0) queda excluido.
		if j != 0 {
			modelo.Pesos[j] *=
				(1 - o.Tasa*weightDecay)
		}
	}

	return nil
}
