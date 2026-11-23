package distribucion

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"grupo5-prograconcurrente/modelocontrolado"
)

type TipoMensaje string

const (
	TipoRegistro   TipoMensaje = "REGISTER"
	TipoRegistroOK TipoMensaje = "REGISTER_ACK"
	TipoEntrenar   TipoMensaje = "TRAIN"
	TipoGradiente  TipoMensaje = "GRADIENT_RESULT"
	TipoError      TipoMensaje = "ERROR"
	TipoDetener    TipoMensaje = "STOP"
)

const (
	VersionProtocolo        = 1
	MaxFrameSize     uint32 = 64 * 1024 * 1024
)

// Mensaje representa cualquier comunicación entre
// el coordinador y los workers.
type Mensaje struct {
	Version int         `json:"version"`
	Tipo    TipoMensaje `json:"tipo"`

	WorkerID string `json:"worker_id,omitempty"`
	JobID    string `json:"job_id,omitempty"`

	Epoch  int `json:"epoch,omitempty"`
	Batch  int `json:"batch,omitempty"`
	Inicio int `json:"inicio,omitempty"`
	Fin    int `json:"fin,omitempty"`

	RegistrosDisponibles int `json:"registros_disponibles,omitempty"`

	Pesos          []float64 `json:"pesos,omitempty"`
	PesoPositivo   float64   `json:"peso_positivo,omitempty"`
	UsaDescripcion bool      `json:"usa_descripcion,omitempty"`

	Gradiente  []float64 `json:"gradiente,omitempty"`
	SumaPesos  float64   `json:"suma_pesos,omitempty"`
	Muestras   int       `json:"muestras,omitempty"`
	Positivos  int       `json:"positivos,omitempty"`
	DuracionMs int64     `json:"duracion_ms,omitempty"`

	Aceptado bool   `json:"aceptado,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Validar comprueba la estructura de cada mensaje.
func (m Mensaje) Validar() error {
	if m.Version != VersionProtocolo {
		return fmt.Errorf("versión de protocolo incompatible")
	}

	switch m.Tipo {

	case TipoRegistro:
		if strings.TrimSpace(m.WorkerID) == "" {
			return fmt.Errorf("worker_id obligatorio")
		}
		if m.RegistrosDisponibles < 0 {
			return fmt.Errorf("cantidad de registros inválida")
		}

	case TipoRegistroOK:
		if strings.TrimSpace(m.WorkerID) == "" {
			return fmt.Errorf("worker_id obligatorio")
		}

	case TipoEntrenar:
		if m.JobID == "" || m.Epoch < 1 || m.Batch < 0 {
			return fmt.Errorf("identificación de entrenamiento inválida")
		}

		if m.Inicio < 0 || m.Fin <= m.Inicio {
			return fmt.Errorf("rango de procesamiento inválido")
		}

		if len(m.Pesos) != modelocontrolado.DimensionModelo {
			return fmt.Errorf("dimensiones de pesos incorrectas")
		}

		if m.PesoPositivo <= 0 ||
			math.IsNaN(m.PesoPositivo) ||
			math.IsInf(m.PesoPositivo, 0) {
			return fmt.Errorf("peso positivo inválido")
		}

		if err := validarVector(m.Pesos); err != nil {
			return err
		}

	case TipoGradiente:
		if m.WorkerID == "" || m.JobID == "" {
			return fmt.Errorf("identificación del resultado incompleta")
		}

		if m.Epoch < 1 || m.Batch < 0 {
			return fmt.Errorf("iteración inválida")
		}

		if len(m.Gradiente) != modelocontrolado.DimensionModelo {
			return fmt.Errorf("dimensiones del gradiente incorrectas")
		}

		if m.Muestras <= 0 ||
			m.SumaPesos <= 0 ||
			math.IsNaN(m.SumaPesos) ||
			math.IsInf(m.SumaPesos, 0) {
			return fmt.Errorf("resultado de gradiente inválido")
		}

		if m.Positivos < 0 || m.Positivos > m.Muestras {
			return fmt.Errorf("cantidad de positivos inválida")
		}

		if err := validarVector(m.Gradiente); err != nil {
			return err
		}

	case TipoError:
		if strings.TrimSpace(m.Error) == "" {
			return fmt.Errorf("mensaje de error vacío")
		}

	case TipoDetener:
		// No requiere campos adicionales.

	default:
		return fmt.Errorf("tipo de mensaje desconocido: %s", m.Tipo)
	}

	return nil
}

func validarVector(valores []float64) error {
	for i, valor := range valores {
		if math.IsNaN(valor) || math.IsInf(valor, 0) {
			return fmt.Errorf("valor no válido en posición %d", i)
		}
	}
	return nil
}

// escribirCompleto garantiza que se escriban todos los bytes.
func escribirCompleto(w io.Writer, datos []byte) error {
	for len(datos) > 0 {
		n, err := w.Write(datos)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		datos = datos[n:]
	}
	return nil
}

// EscribirMensaje serializa y transmite un mensaje TCP.
// Formato: 4 bytes de longitud + contenido JSON.
func EscribirMensaje(w io.Writer, m Mensaje) error {
	if err := m.Validar(); err != nil {
		return err
	}

	contenido, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("error serializando mensaje: %w", err)
	}

	if len(contenido) == 0 || uint64(len(contenido)) > uint64(MaxFrameSize) {
		return fmt.Errorf("tamaño de mensaje inválido")
	}

	var cabecera [4]byte
	binary.BigEndian.PutUint32(cabecera[:], uint32(len(contenido)))

	if err := escribirCompleto(w, cabecera[:]); err != nil {
		return err
	}

	return escribirCompleto(w, contenido)
}

// LeerMensaje recibe un mensaje TCP completo.
func LeerMensaje(r io.Reader) (Mensaje, error) {
	var mensaje Mensaje
	var cabecera [4]byte

	if _, err := io.ReadFull(r, cabecera[:]); err != nil {
		return mensaje, err
	}

	tamano := binary.BigEndian.Uint32(cabecera[:])

	if tamano == 0 || tamano > MaxFrameSize {
		return mensaje, fmt.Errorf("tamaño de mensaje TCP inválido")
	}

	contenido := make([]byte, tamano)

	if _, err := io.ReadFull(r, contenido); err != nil {
		return mensaje, err
	}

	if err := json.Unmarshal(contenido, &mensaje); err != nil {
		return mensaje, fmt.Errorf("JSON inválido: %w", err)
	}

	if err := mensaje.Validar(); err != nil {
		return mensaje, err
	}

	return mensaje, nil
}
