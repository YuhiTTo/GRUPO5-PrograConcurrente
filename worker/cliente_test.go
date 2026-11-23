package worker

import (
	"fmt"
	"testing"
	"time"

	"grupo5-prograconcurrente/distribucion"
	"grupo5-prograconcurrente/modelocontrolado"
)

func TestEjecutarTCPCompletaCiclo(t *testing.T) {

	listener, err := distribucion.EscucharTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	resultadoServidor := make(chan error, 1)

	// Simular el coordinador.
	go func() {

		conn, err := listener.Accept()
		if err != nil {
			resultadoServidor <- err
			return
		}
		defer conn.Close()

		// Recibir registro del worker.
		registro, err := distribucion.RecibirMensajeTCP(
			conn,
			5*time.Second,
		)
		if err != nil {
			resultadoServidor <- err
			return
		}

		if registro.Tipo != distribucion.TipoRegistro {
			resultadoServidor <- fmt.Errorf("registro incorrecto")
			return
		}

		if registro.WorkerID != "worker-1" {
			resultadoServidor <- fmt.Errorf("worker incorrecto")
			return
		}

		if registro.RegistrosDisponibles != 8 {
			resultadoServidor <- fmt.Errorf("cantidad de registros incorrecta")
			return
		}

		// Aceptar registro.
		ack := distribucion.Mensaje{
			Version:  distribucion.VersionProtocolo,
			Tipo:     distribucion.TipoRegistroOK,
			WorkerID: "worker-1",
			Aceptado: true,
		}

		if err := distribucion.EnviarMensajeTCP(
			conn,
			ack,
			5*time.Second,
		); err != nil {
			resultadoServidor <- err
			return
		}

		// Enviar trabajo al worker.
		trabajo := distribucion.Mensaje{
			Version: distribucion.VersionProtocolo,
			Tipo:    distribucion.TipoEntrenar,
			JobID:   "job-001",
			Epoch:   1,
			Batch:   0,
			Inicio:  2,
			Fin:     5,
			Pesos: make(
				[]float64,
				modelocontrolado.DimensionModelo,
			),
			PesoPositivo:   2.0,
			UsaDescripcion: true,
		}

		if err := distribucion.EnviarMensajeTCP(
			conn,
			trabajo,
			5*time.Second,
		); err != nil {
			resultadoServidor <- err
			return
		}

		// Recibir gradiente calculado.
		gradiente, err := distribucion.RecibirMensajeTCP(
			conn,
			5*time.Second,
		)
		if err != nil {
			resultadoServidor <- err
			return
		}

		if gradiente.Tipo != distribucion.TipoGradiente {
			resultadoServidor <- fmt.Errorf("gradiente incorrecto")
			return
		}

		if gradiente.Muestras != 3 {
			resultadoServidor <- fmt.Errorf("cantidad de muestras incorrecta")
			return
		}

		if gradiente.Positivos < 0 ||
			gradiente.Positivos > gradiente.Muestras {
			resultadoServidor <- fmt.Errorf(
				"cantidad de positivos incorrecta",
			)
			return
		}

		// Finalizar el worker.
		detener := distribucion.Mensaje{
			Version: distribucion.VersionProtocolo,
			Tipo:    distribucion.TipoDetener,
		}

		if err := distribucion.EnviarMensajeTCP(
			conn,
			detener,
			5*time.Second,
		); err != nil {
			resultadoServidor <- err
			return
		}

		resultadoServidor <- nil
	}()

	// Preparar datos locales.
	datos := crearDatosPrueba()

	procesador, err := NuevoProcesador("worker-1", datos)
	if err != nil {
		t.Fatal(err)
	}

	// Ejecutar el cliente TCP.
	err = EjecutarTCP(listener.Addr().String(), procesador)
	if err != nil {
		t.Fatal(err)
	}

	// Comprobar el resultado del servidor.
	select {
	case err := <-resultadoServidor:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("tiempo de espera agotado")
	}
}

func TestEjecutarTCPRechazaRegistro(t *testing.T) {

	listener, err := distribucion.EscucharTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		_, err = distribucion.RecibirMensajeTCP(
			conn,
			5*time.Second,
		)
		if err != nil {
			return
		}

		_ = distribucion.EnviarMensajeTCP(
			conn,
			distribucion.Mensaje{
				Version:  distribucion.VersionProtocolo,
				Tipo:     distribucion.TipoRegistroOK,
				WorkerID: "worker-1",
				Aceptado: false,
			},
			5*time.Second,
		)
	}()

	procesador, err := NuevoProcesador(
		"worker-1",
		crearDatosPrueba(),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = EjecutarTCP(listener.Addr().String(), procesador)

	if err == nil {
		t.Fatal("se esperaba rechazo del coordinador")
	}
}

func TestEjecutarTCPRechazaProcesadorNulo(t *testing.T) {
	err := EjecutarTCP("127.0.0.1:9000", nil)

	if err == nil {
		t.Fatal("se esperaba error por procesador nulo")
	}
}
