package distribucion

import (
	"fmt"
	"testing"
	"time"
)

func TestTCPIntercambioMensajes(t *testing.T) {

	// Iniciar servidor en un puerto disponible.
	listener, err := EscucharTCP("127.0.0.1:0")
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
		mensaje, err := RecibirMensajeTCP(conn, 5*time.Second)
		if err != nil {
			resultadoServidor <- err
			return
		}

		if mensaje.Tipo != TipoRegistro {
			resultadoServidor <- fmt.Errorf(
				"se esperaba REGISTER, se recibió %s",
				mensaje.Tipo,
			)
			return
		}

		// Responder confirmando el registro.
		respuesta := Mensaje{
			Version:  VersionProtocolo,
			Tipo:     TipoRegistroOK,
			WorkerID: mensaje.WorkerID,
			Aceptado: true,
		}

		resultadoServidor <- EnviarMensajeTCP(
			conn,
			respuesta,
			5*time.Second,
		)
	}()

	// Simular un worker.
	conn, err := ConectarTCP(
		listener.Addr().String(),
		5*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	registro := Mensaje{
		Version:              VersionProtocolo,
		Tipo:                 TipoRegistro,
		WorkerID:             "worker-1",
		RegistrosDisponibles: 1000,
	}

	// Enviar registro.
	if err := EnviarMensajeTCP(
		conn,
		registro,
		5*time.Second,
	); err != nil {
		t.Fatal(err)
	}

	// Recibir confirmación.
	respuesta, err := RecibirMensajeTCP(
		conn,
		5*time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}

	if respuesta.Tipo != TipoRegistroOK {
		t.Fatal("respuesta incorrecta")
	}

	if respuesta.WorkerID != "worker-1" {
		t.Fatal("identificador de worker incorrecto")
	}

	if !respuesta.Aceptado {
		t.Fatal("el registro no fue aceptado")
	}

	if err := <-resultadoServidor; err != nil {
		t.Fatal(err)
	}
}

func TestEnviarMensajeTCPRechazaConexionNula(t *testing.T) {
	err := EnviarMensajeTCP(
		nil,
		Mensaje{},
		time.Second,
	)

	if err == nil {
		t.Fatal("se esperaba error por conexión nula")
	}
}

func TestRecibirMensajeTCPRechazaConexionNula(t *testing.T) {
	_, err := RecibirMensajeTCP(nil, time.Second)

	if err == nil {
		t.Fatal("se esperaba error por conexión nula")
	}
}
