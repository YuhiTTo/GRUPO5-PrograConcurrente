package worker

import (
	"fmt"
	"strings"
	"time"

	"grupo5-prograconcurrente/distribucion"
	"grupo5-prograconcurrente/modelocontrolado"
)

// Procesador representa el motor de cálculo local de un worker.
type Procesador struct {
	ID         string
	Datos      []modelocontrolado.Ejemplo
	Goroutines int
}

// NuevoProcesador inicializa un worker con sus datos locales.
func NuevoProcesador(
	id string,
	datos []modelocontrolado.Ejemplo,
) (*Procesador, error) {

	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("identificador de worker vacío")
	}

	if len(datos) == 0 {
		return nil, fmt.Errorf("el worker no tiene datos disponibles")
	}

	return &Procesador{
		ID:         id,
		Datos:      datos,
		Goroutines: 1,
	}, nil
}

func (p *Procesador) ConfigurarGoroutines(cantidad int) error {
	if p == nil {
		return fmt.Errorf("procesador no inicializado")
	}

	if cantidad < 1 {
		return fmt.Errorf("la cantidad de goroutines debe ser mayor que cero")
	}

	p.Goroutines = cantidad
	return nil
}

// Procesar calcula el gradiente del segmento asignado
// utilizando los pesos globales enviados por el coordinador.
func (p *Procesador) Procesar(
	mensaje distribucion.Mensaje,
) (distribucion.Mensaje, error) {

	if p == nil {
		return distribucion.Mensaje{},
			fmt.Errorf("procesador no inicializado")
	}

	if strings.TrimSpace(p.ID) == "" {
		return distribucion.Mensaje{},
			fmt.Errorf("identificador de worker vacío")
	}

	if err := mensaje.Validar(); err != nil {
		return distribucion.Mensaje{}, fmt.Errorf(
			"mensaje de entrenamiento inválido: %w", err,
		)
	}

	if mensaje.Tipo != distribucion.TipoEntrenar {
		return distribucion.Mensaje{},
			fmt.Errorf("se esperaba un mensaje TRAIN")
	}

	// Comprobar que el segmento pertenece a los datos disponibles.
	if mensaje.Inicio < 0 ||
		mensaje.Fin > len(p.Datos) ||
		mensaje.Fin <= mensaje.Inicio {

		return distribucion.Mensaje{},
			fmt.Errorf("rango de procesamiento fuera de límites")
	}

	inicio := time.Now()

	// Construir el modelo temporal con los pesos globales.
	modelo := &modelocontrolado.Modelo{
		Pesos:          mensaje.Pesos,
		PesoPositivo:   mensaje.PesoPositivo,
		UsaDescripcion: mensaje.UsaDescripcion,
	}

	// Obtener exclusivamente los registros asignados.
	subconjunto := p.Datos[mensaje.Inicio:mensaje.Fin]

	// Calcular el gradiente local.
	// Calcular el gradiente local.
	var parcial modelocontrolado.GradienteParcial
	var err error

	if p.Goroutines > 1 {
		parcial, err = modelo.CalcularGradienteParalelo(
			subconjunto,
			p.Goroutines,
		)
	} else {
		parcial, err = modelo.CalcularGradiente(subconjunto)
	}

	if err != nil {
		return distribucion.Mensaje{}, fmt.Errorf(
			"error calculando gradiente: %w", err,
		)
	}

	// Preparar la respuesta para el coordinador.
	respuesta := distribucion.Mensaje{
		Version:    distribucion.VersionProtocolo,
		Tipo:       distribucion.TipoGradiente,
		WorkerID:   p.ID,
		JobID:      mensaje.JobID,
		Epoch:      mensaje.Epoch,
		Batch:      mensaje.Batch,
		Gradiente:  parcial.Gradiente,
		SumaPesos:  parcial.SumaPesos,
		Muestras:   parcial.Muestras,
		Positivos:  parcial.Positivos,
		DuracionMs: time.Since(inicio).Milliseconds(),
	}

	if err := respuesta.Validar(); err != nil {
		return distribucion.Mensaje{}, fmt.Errorf(
			"respuesta de gradiente inválida: %w", err,
		)
	}

	return respuesta, nil
}
