package datos

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"grupo5-prograconcurrente/modelocontrolado"
)

// Estadisticas resume la lectura del dataset.
type Estadisticas struct {
	FilasLeidas      int
	FueraPeriodo     int
	Invalidos        int
	Registros        int
	Clase0           int
	Clase1           int
	MetodoVacio      int
	DescripcionVacia int
}

func indicesColumnas(cabecera []string) map[string]int {
	indices := make(map[string]int)

	for i, nombre := range cabecera {
		nombre = strings.TrimPrefix(
			strings.TrimSpace(nombre),
			"\ufeff",
		)
		indices[nombre] = i
	}

	return indices
}

// CargarEntrenamientoCSV carga y codifica exclusivamente
// los registros comprendidos entre los años indicados.
func CargarEntrenamientoCSV(
	ruta string,
	anioInicio int,
	anioFin int,
	usarDescripcion bool,
) ([]modelocontrolado.Ejemplo, Estadisticas, error) {

	var stats Estadisticas

	if anioInicio < 2004 || anioFin > 2026 ||
		anioInicio > anioFin {
		return nil, stats, fmt.Errorf("rango de años inválido")
	}

	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, stats, fmt.Errorf(
			"no se pudo abrir el CSV: %w", err,
		)
	}
	defer archivo.Close()

	reader := csv.NewReader(
		bufio.NewReaderSize(archivo, 1<<20),
	)
	reader.ReuseRecord = true

	cabecera, err := reader.Read()
	if err != nil {
		return nil, stats, fmt.Errorf(
			"no se pudo leer la cabecera: %w", err,
		)
	}

	reader.FieldsPerRecord = len(cabecera)

	indices := indicesColumnas(cabecera)

	requeridas := []string{
		"ocid",
		"metodo",
		"categoria",
		"fecha_publicacion",
		"buyer_id",
		"metodo_detalle",
		"titulo",
		"descripcion",
		"riesgo",
	}

	for _, nombre := range requeridas {
		if _, ok := indices[nombre]; !ok {
			return nil, stats, fmt.Errorf(
				"falta la columna %q", nombre,
			)
		}
	}

	datos := make(
		[]modelocontrolado.Ejemplo,
		0,
		400000,
	)

	for {
		fila, err := reader.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, stats, fmt.Errorf(
				"error leyendo registro %d: %w",
				stats.FilasLeidas+1,
				err,
			)
		}

		stats.FilasLeidas++

		fecha := strings.TrimSpace(
			fila[indices["fecha_publicacion"]],
		)

		if len(fecha) < 7 {
			stats.Invalidos++
			continue
		}

		anio, err := strconv.Atoi(fecha[:4])
		if err != nil {
			stats.Invalidos++
			continue
		}

		// Excluir validación y prueba.
		if anio < anioInicio || anio > anioFin {
			stats.FueraPeriodo++
			continue
		}

		ocid := strings.TrimSpace(
			fila[indices["ocid"]],
		)

		riesgo := strings.TrimSpace(
			fila[indices["riesgo"]],
		)

		if ocid == "" ||
			(riesgo != "0" && riesgo != "1") {
			stats.Invalidos++
			continue
		}

		clase := uint8(riesgo[0] - '0')

		registro := modelocontrolado.Registro{
			OCID:             ocid,
			Metodo:           fila[indices["metodo"]],
			Categoria:        fila[indices["categoria"]],
			FechaPublicacion: fecha,
			BuyerID:          fila[indices["buyer_id"]],
			MetodoDetalle:    fila[indices["metodo_detalle"]],
			Titulo:           fila[indices["titulo"]],
			Descripcion:      fila[indices["descripcion"]],
		}

		ejemplo, err :=
			modelocontrolado.CodificarEtiquetado(
				registro,
				clase,
				usarDescripcion,
			)

		if err != nil {
			stats.Invalidos++
			continue
		}

		if strings.TrimSpace(registro.Metodo) == "" {
			stats.MetodoVacio++
		}

		if strings.TrimSpace(registro.Descripcion) == "" {
			stats.DescripcionVacia++
		}

		datos = append(datos, ejemplo)

		stats.Registros++

		if clase == 0 {
			stats.Clase0++
		} else {
			stats.Clase1++
		}
	}

	return datos, stats, nil
}
