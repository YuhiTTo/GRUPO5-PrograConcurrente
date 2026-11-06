package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	completo uint8 = 1 << iota
	noExitoso
	cancelado
	activo
	sinEstado
	desconocido
)

func main() {
	inicio := time.Now()

	archivo, err := os.Open("data/tender_items.csv")
	if err != nil {
		panic(err)
	}
	defer archivo.Close()

	lector := csv.NewReader(
		bufio.NewReaderSize(archivo, 1024*1024),
	)
	lector.ReuseRecord = true

	columnas, err := lector.Read()
	if err != nil {
		panic(err)
	}

	indiceOcid := -1
	indiceEstado := -1

	for i, columna := range columnas {
		switch columna {
		case "main_ocid":
			indiceOcid = i
		case "status":
			indiceEstado = i
		}
	}

	if indiceOcid < 0 || indiceEstado < 0 {
		panic("No se encontraron las columnas requeridas")
	}

	// Cada procedimiento almacena los estados de sus items.
	estadosPorProceso := make(map[string]uint8)

	var total int64

	for {
		fila, err := lector.Read()

		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}

		ocid := strings.TrimSpace(fila[indiceOcid])
		estado := strings.TrimSpace(fila[indiceEstado])

		if ocid == "" {
			continue
		}

		var bandera uint8

		switch estado {
		case "complete":
			bandera = completo
		case "unsuccessful":
			bandera = noExitoso
		case "cancelled":
			bandera = cancelado
		case "active":
			bandera = activo
		case "":
			bandera = sinEstado
		default:
			bandera = desconocido
		}

		estadosPorProceso[ocid] |= bandera
		total++
	}

	var soloCompletos int64
	var soloDesfavorables int64
	var mixtos int64
	var indeterminados int64

	for _, estados := range estadosPorProceso {

		if estados&(activo|sinEstado|desconocido) != 0 {
			indeterminados++
			continue
		}

		tieneCompleto := estados&completo != 0
		tieneDesfavorable := estados&(noExitoso|cancelado) != 0

		switch {
		case tieneCompleto && tieneDesfavorable:
			mixtos++
		case tieneCompleto:
			soloCompletos++
		case tieneDesfavorable:
			soloDesfavorables++
		default:
			indeterminados++
		}
	}

	fmt.Println("\n=== ANALISIS DE PROCEDIMIENTOS ===")
	fmt.Println("Items procesados:", total)
	fmt.Println("Procedimientos unicos:", len(estadosPorProceso))
	fmt.Println("Solo completos:", soloCompletos)
	fmt.Println("Solo desfavorables:", soloDesfavorables)
	fmt.Println("Resultados mixtos:", mixtos)
	fmt.Println("Indeterminados:", indeterminados)
	fmt.Println("Tiempo:", time.Since(inicio))
}
