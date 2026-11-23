package modelocontrolado

import (
	"math"
	"testing"
)

func TestAdamWReplicaActualizacionPC3(t *testing.T) {

	modelo := &Modelo{
		Pesos: make([]float64, DimensionModelo),
	}

	modelo.Pesos[0] = 1.0
	modelo.Pesos[1] = 1.0

	opt, err := NuevoOptimizadorAdamW(0.03)
	if err != nil {
		t.Fatal(err)
	}

	grad := GradienteParcial{
		Gradiente: make([]float64, DimensionModelo),
		SumaPesos: 2,
		Muestras:  2,
	}

	grad.Gradiente[0] = 2
	grad.Gradiente[1] = 4

	err = opt.Actualizar(modelo, grad)
	if err != nil {
		t.Fatal(err)
	}

	stepIntercepto := 0.03 / (1.0 + 1e-8)

	stepPeso := 0.03 * 2.0 / (2.0 + 1e-8)

	esperadoIntercepto := 1.0 - stepIntercepto

	esperadoPeso := (1.0 - stepPeso) *
		(1 - 0.03*0.0005)

	if math.Abs(
		modelo.Pesos[0]-esperadoIntercepto,
	) > 1e-12 {
		t.Errorf("Intercepto incorrecto")
	}

	if math.Abs(
		modelo.Pesos[1]-esperadoPeso,
	) > 1e-12 {
		t.Errorf("Peso con AdamW incorrecto")
	}

	if opt.Iteracion != 1 {
		t.Errorf("Iteración incorrecta")
	}
}

func TestAdamWRechazaGradienteInvalido(t *testing.T) {

	modelo := &Modelo{
		Pesos: make([]float64, DimensionModelo),
	}

	opt, err := NuevoOptimizadorAdamW(0.03)
	if err != nil {
		t.Fatal(err)
	}

	grad := GradienteParcial{
		Gradiente: make([]float64, DimensionModelo),
		SumaPesos: 0,
		Muestras:  1,
	}

	err = opt.Actualizar(modelo, grad)

	if err == nil {
		t.Fatal("Se esperaba error para gradiente inválido")
	}
}
