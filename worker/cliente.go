package worker

import (
	"fmt"
	"math/rand"

	"grupo5-prograconcurrente/datos"
	"grupo5-prograconcurrente/distribucion"
)

// EjecutarTCP conecta un worker con el coordinador
// y procesa las instrucciones recibidas mediante TCP.
func EjecutarTCP(
	direccion string,
	procesador *Procesador,
) error {

	if procesador == nil {
		return fmt.Errorf("procesador no inicializado")
	}

	// Establecer conexión con el coordinador.
	conn, err := distribucion.ConectarTCP(
		direccion,
		distribucion.TimeoutConexion,
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Registrar el worker.
	registro := distribucion.Mensaje{
		Version:              distribucion.VersionProtocolo,
		Tipo:                 distribucion.TipoRegistro,
		WorkerID:             procesador.ID,
		RegistrosDisponibles: len(procesador.Datos),
	}

	if err := distribucion.EnviarMensajeTCP(
		conn,
		registro,
		distribucion.TimeoutMensaje,
	); err != nil {
		return fmt.Errorf("error registrando worker: %w", err)
	}

	// Esperar confirmación del coordinador.
	respuesta, err := distribucion.RecibirMensajeTCP(
		conn,
		distribucion.TimeoutMensaje,
	)
	if err != nil {
		return fmt.Errorf("error recibiendo confirmación: %w", err)
	}

	if respuesta.Tipo != distribucion.TipoRegistroOK {
		return fmt.Errorf("respuesta de registro inesperada")
	}

	if respuesta.WorkerID != procesador.ID {
		return fmt.Errorf("identificador de worker incorrecto")
	}

	if !respuesta.Aceptado {
		return fmt.Errorf("el coordinador rechazó el registro")
	}

	// Reproducir exactamente la secuencia de barajado de PC3.
	rng := rand.New(
		rand.NewSource(datos.SemillaEntrenamiento),
	)

	ultimaEpoca := 0

	// Bucle de procesamiento.
	for {
		mensaje, err := distribucion.RecibirMensajeTCP(
			conn,
			distribucion.TimeoutEsperaTrabajo,
		)
		if err != nil {
			return fmt.Errorf("error recibiendo instrucción: %w", err)
		}

		switch mensaje.Tipo {

		case distribucion.TipoDetener:
			return nil

		case distribucion.TipoError:
			return fmt.Errorf(
				"error informado por el coordinador: %s",
				mensaje.Error,
			)

		case distribucion.TipoEntrenar:

			// Sincronizar el barajado al comenzar cada época.
			if mensaje.Epoch != ultimaEpoca {

				if mensaje.Epoch != ultimaEpoca+1 {
					return fmt.Errorf(
						"secuencia de épocas inválida: recibida %d, esperada %d",
						mensaje.Epoch,
						ultimaEpoca+1,
					)
				}

				if err := datos.BarajarEpoca(
					procesador.Datos,
					rng,
				); err != nil {
					return fmt.Errorf(
						"error barajando datos: %w",
						err,
					)
				}

				ultimaEpoca = mensaje.Epoch
			}

			// Calcular gradiente local.
			resultado, err := procesador.Procesar(mensaje)

			if err != nil {
				errorMensaje := distribucion.Mensaje{
					Version:  distribucion.VersionProtocolo,
					Tipo:     distribucion.TipoError,
					WorkerID: procesador.ID,
					JobID:    mensaje.JobID,
					Error:    err.Error(),
				}

				if envioErr := distribucion.EnviarMensajeTCP(
					conn,
					errorMensaje,
					distribucion.TimeoutMensaje,
				); envioErr != nil {
					return fmt.Errorf(
						"error procesando trabajo: %v; error notificando: %w",
						err,
						envioErr,
					)
				}

				return fmt.Errorf("error procesando trabajo: %w", err)
			}

			// Devolver el gradiente al coordinador.
			if err := distribucion.EnviarMensajeTCP(
				conn,
				resultado,
				distribucion.TimeoutMensaje,
			); err != nil {
				return fmt.Errorf("error enviando gradiente: %w", err)
			}

		default:
			return fmt.Errorf(
				"tipo de instrucción no soportado: %s",
				mensaje.Tipo,
			)
		}
	}
}
