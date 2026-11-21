package modelocontrolado

import (
	"math"
	"testing"
)

func TestGradienteParaleloCoincideConSecuencial(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	registros := []struct {
		registro Registro
		clase    uint8
	}{
		{
			registro: Registro{
				Metodo:           "OPEN",
				Categoria:        "SERVICIOS",
				FechaPublicacion: "2025-06-15",
				BuyerID:          "PE-ENTIDAD-001",
				MetodoDetalle:    "LICITACION PUBLICA",
				Titulo:           "Adquisicion de equipos informaticos",
				Descripcion:      "Compra de equipos tecnologicos",
			},
			clase: 0,
		},
		{
			registro: Registro{
				Metodo:           "DIRECT",
				Categoria:        "BIENES",
				FechaPublicacion: "2024-11-20",
				BuyerID:          "PE-ENTIDAD-002",
				MetodoDetalle:    "CONTRATACION DIRECTA",
				Titulo:           "Compra de materiales medicos",
				Descripcion:      "Adquisicion de materiales hospitalarios",
			},
			clase: 1,
		},
		{
			registro: Registro{
				Metodo:           "OPEN",
				Categoria:        "OBRAS",
				FechaPublicacion: "2023-03-10",
				BuyerID:          "PE-ENTIDAD-003",
				MetodoDetalle:    "LICITACION PUBLICA",
				Titulo:           "Construccion de infraestructura",
				Descripcion:      "Ejecucion de obras publicas",
			},
			clase: 0,
		},
	}

	ejemplos := make([]Ejemplo, 0, len(registros))

	for _, item := range registros {
		e, err := CodificarEtiquetado(
			item.registro,
			item.clase,
			modelo.UsaDescripcion,
		)

		if err != nil {
			t.Fatal(err)
		}

		ejemplos = append(ejemplos, e)
	}

	secuencial, err := modelo.CalcularGradiente(ejemplos)
	if err != nil {
		t.Fatal(err)
	}

	paralelo, err := modelo.CalcularGradienteParalelo(
		ejemplos, 2,
	)
	if err != nil {
		t.Fatal(err)
	}

	if secuencial.Muestras != paralelo.Muestras {
		t.Fatal("Cantidad de muestras diferente")
	}

	if secuencial.Positivos != paralelo.Positivos {
		t.Fatal("Cantidad de positivos diferente")
	}

	if math.Abs(secuencial.SumaPesos-paralelo.SumaPesos) > 1e-12 {
		t.Fatal("Suma de pesos diferente")
	}

	for i := 0; i < DimensionModelo; i++ {
		diferencia := math.Abs(
			secuencial.Gradiente[i] -
				paralelo.Gradiente[i],
		)

		if diferencia > 1e-10 {
			t.Fatalf(
				"Gradiente diferente en índice %d: diferencia %g",
				i, diferencia,
			)
		}
	}
}

func TestGradienteRechazaMiniBatchVacio(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	_, err := modelo.CalcularGradiente(nil)

	if err == nil {
		t.Fatal("Se esperaba error para mini-batch vacío")
	}
}

func TestGradienteRechazaWorkersInvalidos(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	_, err := modelo.CalcularGradienteParalelo(nil, 0)

	if err == nil {
		t.Fatal("Se esperaba error para cero workers")
	}
}
