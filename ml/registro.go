// Package ml implementa la regresión logística con entrenamiento paralelo
// (paralelismo de datos) para predecir el riesgo de que un procedimiento de
// contratación pública termine cancelado o desierto.
package ml

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Registro representa un procedimiento de contratación etiquetado,
// tal como lo genera preparar_dataset.go en data/entrenamiento_base.csv.
type Registro struct {
	OCID            string
	Monto           float64 // monto referencial en PEN; 0 si no está disponible
	MontoDisponible bool
	Metodo          string
	Categoria       string
	Anio            int     // 0 si la fecha no es válida
	Mes             int     // 1..12; 0 si la fecha no es válida
	Riesgo          float64 // 0 = completado, 1 = cancelado o desierto

	MetodoDetalle      string  // tipo de procedimiento normalizado, por ejemplo "LICITACION PUBLICA"
	Duracion           float64 // días de convocatoria; 0 si no está disponible
	DuracionDisponible bool
}

// ColumnasEntrenamiento son las columnas que debe tener el CSV de entrenamiento.
var ColumnasEntrenamiento = []string{
	"ocid",
	"monto_pen",
	"monto_disponible",
	"metodo",
	"categoria",
	"fecha_publicacion",
	"riesgo",
	"metodo_detalle",
	"duracion_convocatoria_dias",
}

// CargarRegistros lee el CSV de entrenamiento desde una ruta.
func CargarRegistros(ruta string) ([]Registro, error) {
	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer archivo.Close()

	return LeerRegistros(archivo)
}

// LeerRegistros lee el CSV de entrenamiento desde cualquier lector.
// Valida el encabezado y rechaza filas con una etiqueta distinta de 0 o 1.
func LeerRegistros(r io.Reader) ([]Registro, error) {
	lector := csv.NewReader(bufio.NewReaderSize(r, 1024*1024))
	lector.ReuseRecord = true

	encabezado, err := lector.Read()
	if err != nil {
		return nil, fmt.Errorf("leer encabezado: %w", err)
	}

	indices := make(map[string]int, len(encabezado))
	for i, columna := range encabezado {
		indices[strings.TrimSpace(columna)] = i
	}

	posiciones := make([]int, len(ColumnasEntrenamiento))
	for i, columna := range ColumnasEntrenamiento {
		j, existe := indices[columna]
		if !existe {
			return nil, fmt.Errorf("falta la columna %q", columna)
		}
		posiciones[i] = j
	}

	// Los métodos y categorías se repiten millones de veces: se internan
	// para no guardar una copia del texto por cada fila.
	internados := make(map[string]string)
	internar := func(s string) string {
		if v, existe := internados[s]; existe {
			return v
		}
		v := strings.Clone(s)
		internados[v] = v
		return v
	}

	var registros []Registro
	numero := 0

	for {
		fila, err := lector.Read()
		if err == io.EOF {
			break
		}
		numero++
		if err != nil {
			return nil, fmt.Errorf("registro %d: %w", numero, err)
		}

		registro, err := parsearRegistro(fila, posiciones, internar)
		if err != nil {
			return nil, fmt.Errorf("registro %d: %w", numero, err)
		}
		registros = append(registros, registro)
	}

	return registros, nil
}

func parsearRegistro(fila []string, pos []int, internar func(string) string) (Registro, error) {
	var r Registro

	r.OCID = strings.Clone(strings.TrimSpace(fila[pos[0]]))

	switch strings.TrimSpace(fila[pos[6]]) {
	case "0":
		r.Riesgo = 0
	case "1":
		r.Riesgo = 1
	default:
		return r, fmt.Errorf("riesgo inválido %q", fila[pos[6]])
	}

	if strings.TrimSpace(fila[pos[2]]) == "1" {
		monto, err := strconv.ParseFloat(strings.TrimSpace(fila[pos[1]]), 64)
		if err == nil && monto > 0 && !math.IsInf(monto, 0) {
			r.Monto = monto
			r.MontoDisponible = true
		}
	}

	r.Metodo = internar(normalizarCategoria(fila[pos[3]]))
	r.Categoria = internar(normalizarCategoria(fila[pos[4]]))
	r.Anio, r.Mes = parsearFecha(strings.TrimSpace(fila[pos[5]]))
	r.MetodoDetalle = internar(normalizarCategoria(fila[pos[7]]))

	if valor := strings.TrimSpace(fila[pos[8]]); valor != "" {
		dias, err := strconv.ParseFloat(valor, 64)
		if err == nil && dias >= 0 && !math.IsInf(dias, 0) {
			r.Duracion = dias
			r.DuracionDisponible = true
		}
	}

	return r, nil
}

func normalizarCategoria(valor string) string {
	valor = strings.TrimSpace(valor)
	if valor == "" {
		return "DESCONOCIDO"
	}
	return valor
}

// parsearFecha extrae año y mes de fechas ISO 8601 (por ejemplo
// "2021-03-15T10:00:00-05:00"). Devuelve 0, 0 si la fecha no es válida.
func parsearFecha(fecha string) (int, int) {
	if len(fecha) < 7 || fecha[4] != '-' {
		return 0, 0
	}

	anio, errAnio := strconv.Atoi(fecha[:4])
	mes, errMes := strconv.Atoi(fecha[5:7])
	if errAnio != nil || errMes != nil ||
		anio < 1990 || anio > 2100 || mes < 1 || mes > 12 {
		return 0, 0
	}

	return anio, mes
}
