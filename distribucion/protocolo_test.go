package distribucion

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"grupo5-prograconcurrente/modelocontrolado"
)

func TestMensajeRegistroRoundTrip(t *testing.T) {
	original := Mensaje{
		Version:              VersionProtocolo,
		Tipo:                 TipoRegistro,
		WorkerID:             "worker-1",
		RegistrosDisponibles: 1000,
	}

	var buffer bytes.Buffer

	if err := EscribirMensaje(&buffer, original); err != nil {
		t.Fatal(err)
	}

	recibido, err := LeerMensaje(&buffer)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, recibido) {
		t.Fatalf("mensajes diferentes:\nOriginal: %+v\nRecibido: %+v",
			original, recibido)
	}
}

func TestMensajeGradienteRoundTrip(t *testing.T) {
	gradiente := make(
		[]float64,
		modelocontrolado.DimensionModelo,
	)

	for i := range gradiente {
		gradiente[i] = float64(i) * 0.001
	}

	original := Mensaje{
		Version:    VersionProtocolo,
		Tipo:       TipoGradiente,
		WorkerID:   "worker-1",
		JobID:      "job-001",
		Epoch:      1,
		Batch:      0,
		Gradiente:  gradiente,
		SumaPesos:  2.5,
		Muestras:   2,
		Positivos:  1,
		DuracionMs: 150,
	}

	var buffer bytes.Buffer

	if err := EscribirMensaje(&buffer, original); err != nil {
		t.Fatal(err)
	}

	recibido, err := LeerMensaje(&buffer)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(original, recibido) {
		t.Fatal("el gradiente recibido no coincide con el original")
	}
}

func TestEscribirMensajeRechazaVersionInvalida(t *testing.T) {
	mensaje := Mensaje{
		Version:  99,
		Tipo:     TipoRegistro,
		WorkerID: "worker-1",
	}

	var buffer bytes.Buffer

	err := EscribirMensaje(&buffer, mensaje)

	if err == nil {
		t.Fatal("se esperaba error por versión inválida")
	}
}

func TestLeerMensajeRechazaLongitudCero(t *testing.T) {
	var buffer bytes.Buffer

	var cabecera [4]byte
	binary.BigEndian.PutUint32(cabecera[:], 0)

	buffer.Write(cabecera[:])

	_, err := LeerMensaje(&buffer)

	if err == nil {
		t.Fatal("se esperaba error por longitud cero")
	}
}

func TestLeerMensajeRechazaLongitudExcesiva(t *testing.T) {
	var buffer bytes.Buffer

	var cabecera [4]byte
	binary.BigEndian.PutUint32(
		cabecera[:],
		MaxFrameSize+1,
	)

	buffer.Write(cabecera[:])

	_, err := LeerMensaje(&buffer)

	if err == nil {
		t.Fatal("se esperaba error por longitud excesiva")
	}
}

func TestLeerMensajeDetectaMensajeTruncado(t *testing.T) {
	var buffer bytes.Buffer

	var cabecera [4]byte
	binary.BigEndian.PutUint32(cabecera[:], 10)

	buffer.Write(cabecera[:])
	buffer.Write([]byte("hola"))

	_, err := LeerMensaje(&buffer)

	if err == nil {
		t.Fatal("se esperaba error por mensaje incompleto")
	}
}
