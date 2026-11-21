package worker

import (
	"testing"

	"grupo5-prograconcurrente/distribucion"
	"grupo5-prograconcurrente/modelocontrolado"
)

func crearDatosPrueba() []modelocontrolado.Ejemplo {
	datos := make([]modelocontrolado.Ejemplo, 8)

	for i := range datos {
		datos[i] = modelocontrolado.Ejemplo{
			Clase: uint8(i % 2),
		}
	}

	return datos
}

func crearMensajeEntrenamiento() distribucion.Mensaje {
	return distribucion.Mensaje{
		Version:        distribucion.VersionProtocolo,
		Tipo:           distribucion.TipoEntrenar,
		JobID:          "job-001",
		Epoch:          1,
		Batch:          0,
		Inicio:         2,
		Fin:            5,
		Pesos:          make([]float64, modelocontrolado.DimensionModelo),
		PesoPositivo:   2.0,
		UsaDescripcion: true,
	}
}

func TestProcesadorCalculaGradiente(t *testing.T) {
	datos := crearDatosPrueba()

	procesador, err := NuevoProcesador("worker-1", datos)
	if err != nil {
		t.Fatal(err)
	}

	respuesta, err := procesador.Procesar(
		crearMensajeEntrenamiento(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if respuesta.Tipo != distribucion.TipoGradiente {
		t.Fatal("tipo de respuesta incorrecto")
	}

	if respuesta.WorkerID != "worker-1" {
		t.Fatal("identificador de worker incorrecto")
	}

	if respuesta.JobID != "job-001" {
		t.Fatal("identificador de trabajo incorrecto")
	}

	if respuesta.Muestras != 3 {
		t.Fatalf("se esperaban 3 muestras, se recibieron %d",
			respuesta.Muestras)
	}

	if respuesta.Positivos != 1 {
		t.Fatalf("se esperaba 1 positivo, se recibieron %d",
			respuesta.Positivos)
	}

	if respuesta.SumaPesos != 4 {
		t.Fatalf("suma de pesos incorrecta: %f",
			respuesta.SumaPesos)
	}

	if len(respuesta.Gradiente) != modelocontrolado.DimensionModelo {
		t.Fatal("dimensiones del gradiente incorrectas")
	}

	if err := respuesta.Validar(); err != nil {
		t.Fatalf("respuesta inválida: %v", err)
	}
}

func TestProcesadorRechazaRangoFueraDeLimites(t *testing.T) {
	procesador, err := NuevoProcesador(
		"worker-1",
		crearDatosPrueba(),
	)
	if err != nil {
		t.Fatal(err)
	}

	mensaje := crearMensajeEntrenamiento()
	mensaje.Fin = 100

	_, err = procesador.Procesar(mensaje)

	if err == nil {
		t.Fatal("se esperaba error por rango fuera de límites")
	}
}

func TestProcesadorRechazaMensajeIncorrecto(t *testing.T) {
	procesador, err := NuevoProcesador(
		"worker-1",
		crearDatosPrueba(),
	)
	if err != nil {
		t.Fatal(err)
	}

	mensaje := crearMensajeEntrenamiento()
	mensaje.Tipo = distribucion.TipoRegistro

	_, err = procesador.Procesar(mensaje)

	if err == nil {
		t.Fatal("se esperaba error por tipo de mensaje incorrecto")
	}
}

func TestNuevoProcesadorRechazaDatosVacios(t *testing.T) {
	_, err := NuevoProcesador(
		"worker-1",
		nil,
	)

	if err == nil {
		t.Fatal("se esperaba error por datos vacíos")
	}
}

func TestNuevoProcesadorRechazaIDVacio(t *testing.T) {
	_, err := NuevoProcesador(
		"",
		crearDatosPrueba(),
	)

	if err == nil {
		t.Fatal("se esperaba error por ID vacío")
	}
}
