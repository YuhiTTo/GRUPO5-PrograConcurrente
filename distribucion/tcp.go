package distribucion

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	TimeoutConexion      = 5 * time.Second
	TimeoutMensaje       = 30 * time.Second
	TimeoutEsperaTrabajo = 5 * time.Minute
)

// EscucharTCP inicia un servidor TCP en la dirección indicada.
func EscucharTCP(direccion string) (net.Listener, error) {
	if strings.TrimSpace(direccion) == "" {
		return nil, fmt.Errorf("dirección TCP vacía")
	}

	listener, err := net.Listen("tcp", direccion)
	if err != nil {
		return nil, fmt.Errorf("error iniciando servidor TCP: %w", err)
	}

	return listener, nil
}

// ConectarTCP establece una conexión con el coordinador.
func ConectarTCP(
	direccion string,
	timeout time.Duration,
) (net.Conn, error) {

	if strings.TrimSpace(direccion) == "" {
		return nil, fmt.Errorf("dirección TCP vacía")
	}

	if timeout <= 0 {
		timeout = TimeoutConexion
	}

	conn, err := net.DialTimeout("tcp", direccion, timeout)
	if err != nil {
		return nil, fmt.Errorf("error conectando al servidor: %w", err)
	}

	return conn, nil
}

// EnviarMensajeTCP transmite un mensaje mediante TCP.
func EnviarMensajeTCP(
	conn net.Conn,
	mensaje Mensaje,
	timeout time.Duration,
) error {

	if conn == nil {
		return fmt.Errorf("conexión TCP no inicializada")
	}

	if timeout <= 0 {
		timeout = TimeoutMensaje
	}

	if err := conn.SetWriteDeadline(
		time.Now().Add(timeout),
	); err != nil {
		return fmt.Errorf("error estableciendo plazo de escritura: %w", err)
	}

	defer conn.SetWriteDeadline(time.Time{})

	if err := EscribirMensaje(conn, mensaje); err != nil {
		return fmt.Errorf("error enviando mensaje TCP: %w", err)
	}

	return nil
}

// RecibirMensajeTCP recibe un mensaje completo mediante TCP.
func RecibirMensajeTCP(
	conn net.Conn,
	timeout time.Duration,
) (Mensaje, error) {

	var mensaje Mensaje

	if conn == nil {
		return mensaje, fmt.Errorf("conexión TCP no inicializada")
	}

	if timeout <= 0 {
		timeout = TimeoutMensaje
	}

	if err := conn.SetReadDeadline(
		time.Now().Add(timeout),
	); err != nil {
		return mensaje, fmt.Errorf("error estableciendo plazo de lectura: %w", err)
	}

	defer conn.SetReadDeadline(time.Time{})

	mensaje, err := LeerMensaje(conn)
	if err != nil {
		return mensaje, fmt.Errorf("error recibiendo mensaje TCP: %w", err)
	}

	return mensaje, nil
}
