package datos

import (
	"math/rand"
	"reflect"
	"testing"

	"grupo5-prograconcurrente/modelocontrolado"
)

func TestBarajarEpocaDeterminista(t *testing.T) {

	original := make([]modelocontrolado.Ejemplo, 1000)

	for i := range original {
		original[i].Comprador = uint16(i)
		original[i].Clase = uint8(i % 2)
	}

	a := append([]modelocontrolado.Ejemplo{}, original...)
	b := append([]modelocontrolado.Ejemplo{}, original...)

	rngA := rand.New(rand.NewSource(SemillaEntrenamiento))
	rngB := rand.New(rand.NewSource(SemillaEntrenamiento))

	if err := BarajarEpoca(a, rngA); err != nil {
		t.Fatal(err)
	}

	if err := BarajarEpoca(b, rngB); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(a, b) {
		t.Fatal("el barajado no es determinista")
	}

	if reflect.DeepEqual(a, original) {
		t.Fatal("los registros no fueron barajados")
	}
}

func TestBarajarEpocaRechazaGeneradorNulo(t *testing.T) {
	err := BarajarEpoca(
		[]modelocontrolado.Ejemplo{},
		nil,
	)

	if err == nil {
		t.Fatal("se esperaba error por generador nulo")
	}
}

func TestRangoWorkerSinSolapamientos(t *testing.T) {

	tamanos := []int{8192, 8157}

	for _, tamano := range tamanos {

		inicio := 1000
		fin := inicio + tamano
		cursor := inicio

		for worker := 0; worker < 4; worker++ {

			desde, hasta, err := RangoWorker(
				inicio,
				fin,
				worker,
				4,
			)

			if err != nil {
				t.Fatal(err)
			}

			if desde != cursor {
				t.Fatalf("solapamiento o hueco en worker %d", worker)
			}

			if hasta <= desde {
				t.Fatalf("worker %d recibió un segmento vacío", worker)
			}

			cursor = hasta
		}

		if cursor != fin {
			t.Fatalf("no se cubrió todo el mini-lote")
		}
	}
}

func TestRangoWorkerRechazaParametrosInvalidos(t *testing.T) {

	casos := []struct {
		inicio int
		fin    int
		worker int
		total  int
	}{
		{-1, 100, 0, 4},
		{10, 10, 0, 4},
		{0, 100, 0, 0},
		{0, 100, 4, 4},
		{0, 100, -1, 4},
	}

	for _, caso := range casos {

		_, _, err := RangoWorker(
			caso.inicio,
			caso.fin,
			caso.worker,
			caso.total,
		)

		if err == nil {
			t.Fatal("se esperaba error por parámetros inválidos")
		}
	}
}
