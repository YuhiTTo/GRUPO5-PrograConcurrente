package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type Fila struct {
	OCID   string
	Estado string
}

type Resultado struct {
	OCID    string
	Bandera uint8
}

func clasificar(estado string) uint8 {
	switch strings.ToLower(strings.TrimSpace(estado)) {
	case "complete":
		return completo
	case "unsuccessful":
		return noExitoso
	case "cancelled":
		return cancelado
	case "active":
		return activo
	case "":
		return sinEstado
	default:
		return desconocido
	}
}

func worker(jobs <-chan []Fila, resultados chan<- []Resultado, wg *sync.WaitGroup) {
	defer wg.Done()

	for lote := range jobs {
		salida := make([]Resultado, len(lote))

		for i, fila := range lote {
			salida[i] = Resultado{
				OCID:    strings.TrimSpace(fila.OCID),
				Bandera: clasificar(fila.Estado),
			}
		}

		resultados <- salida
	}
}

func main() {
	numWorkers := flag.Int("workers", 4, "Cantidad de workers")
	flag.Parse()

	if *numWorkers < 1 {
		log.Fatal("Debe haber al menos un worker")
	}

	inicio := time.Now()

	archivo, err := os.Open("data/tender_items.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer archivo.Close()

	lector := csv.NewReader(bufio.NewReaderSize(archivo, 1024*1024))
	lector.ReuseRecord = true

	columnas, err := lector.Read()
	if err != nil {
		log.Fatal(err)
	}

	indiceOCID, indiceEstado := -1, -1
	for i, nombre := range columnas {
		switch nombre {
		case "main_ocid":
			indiceOCID = i
		case "status":
			indiceEstado = i
		}
	}

	if indiceOCID < 0 || indiceEstado < 0 {
		log.Fatal("No se encontraron las columnas requeridas")
	}

	jobs := make(chan []Fila, *numWorkers*2)
	resultados := make(chan []Resultado, *numWorkers*2)
	errores := make(chan error, 1)

	var wg sync.WaitGroup

	for i := 0; i < *numWorkers; i++ {
		wg.Add(1)
		go worker(jobs, resultados, &wg)
	}

	// Lectura de lotes de 4096 filas.
	go func() {
		defer close(jobs)

		lote := make([]Fila, 0, 4096)

		for {
			fila, err := lector.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				errores <- err
				return
			}

			lote = append(lote, Fila{
				OCID:   fila[indiceOCID],
				Estado: fila[indiceEstado],
			})

			if len(lote) == 4096 {
				jobs <- lote
				lote = make([]Fila, 0, 4096)
			}
		}

		if len(lote) > 0 {
			jobs <- lote
		}

		errores <- nil
	}()

	go func() {
		wg.Wait()
		close(resultados)
	}()

	// Un solo agregador modifica el mapa.
	// Así evitamos condiciones de carrera.
	estadosPorProceso := make(map[string]uint8)

	var total int64
	var sinOCID int64

	for lote := range resultados {
		for _, r := range lote {
			total++

			if r.OCID == "" {
				sinOCID++
				continue
			}

			estadosPorProceso[r.OCID] |= r.Bandera
		}
	}

	if err := <-errores; err != nil {
		log.Fatal(err)
	}

	rutaSalida := filepath.Join("data", "etiquetas_procesos.csv")
	salida, err := os.Create(rutaSalida)
	if err != nil {
		log.Fatal(err)
	}
	defer salida.Close()

	escritor := csv.NewWriter(salida)

	if err := escritor.Write([]string{"main_ocid", "riesgo"}); err != nil {
		log.Fatal(err)
	}

	var normales, desfavorables, mixtos, indeterminados int64

	for ocid, estados := range estadosPorProceso {
		if estados&(activo|sinEstado|desconocido) != 0 {
			indeterminados++
			continue
		}

		tieneCompleto := estados&completo != 0
		tieneDesfavorable := estados&(noExitoso|cancelado) != 0

		if tieneCompleto && tieneDesfavorable {
			mixtos++
			continue
		}

		etiqueta := ""

		if tieneCompleto {
			etiqueta = "0"
			normales++
		} else if tieneDesfavorable {
			etiqueta = "1"
			desfavorables++
		} else {
			indeterminados++
			continue
		}

		if err := escritor.Write([]string{ocid, etiqueta}); err != nil {
			log.Fatal(err)
		}
	}

	escritor.Flush()
	if err := escritor.Error(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== LIMPIEZA CONCURRENTE ===")
	fmt.Println("Workers utilizados:", *numWorkers)
	fmt.Println("Items procesados:", total)
	fmt.Println("Items sin OCID:", sinOCID)
	fmt.Println("Procedimientos unicos:", len(estadosPorProceso))
	fmt.Println("Clase 0:", normales)
	fmt.Println("Clase 1:", desfavorables)
	fmt.Println("Mixtos excluidos:", mixtos)
	fmt.Println("Indeterminados excluidos:", indeterminados)
	fmt.Println("Archivo generado:", rutaSalida)
	fmt.Println("Tiempo total:", time.Since(inicio))
}
