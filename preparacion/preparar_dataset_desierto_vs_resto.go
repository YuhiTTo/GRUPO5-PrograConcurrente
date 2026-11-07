// Genera un dataset operativo: DESIERTO PURO (1) vs TODOS LOS DEMAS
// resultados etiquetados (0), sin excluir NULO ni CANCELADO por desenlace.
// Fuentes: data/entrenamiento_controlado.csv + data/tender_items.csv.
// Mantiene intactos main.csv, tender_items.csv, entrenamiento_controlado.csv.
// NO demuestra que las caracteristicas de main.csv sean predesenlace.
// Ejecutar desde la raiz: go run .\preparacion\preparar_dataset_desierto_vs_resto.go
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type estado struct {
	items        int
	soloDesierto bool
}

func columna(cab []string, nombre string) int {
	for i, c := range cab {
		if strings.TrimSpace(strings.TrimPrefix(c, "\ufeff")) == nombre {
			return i
		}
	}
	log.Fatalf("Falta la columna %q", nombre)
	return -1
}

func main() {
	inicio := time.Now()
	entrada := "data/entrenamiento_controlado.csv"
	entradaItems := "data/tender_items.csv"
	salida := "data/entrenamiento_desierto_vs_resto.csv"

	f, err := os.Open(entrada)
	if err != nil {
		log.Fatal(err)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}
	idIdx, claseIdx := columna(header, "ocid"), columna(header, "riesgo")

	estados := make(map[string]*estado, 130000)
	totalOriginal := 0
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		totalOriginal++
		if row[claseIdx] == "1" {
			ocid := strings.TrimSpace(row[idIdx])
			if ocid == "" {
				log.Fatalf("OCID vacio en fila %d", totalOriginal+1)
			}
			estados[ocid] = &estado{soloDesierto: true}
		} else if row[claseIdx] != "0" {
			log.Fatalf("Clase inesperada %q en fila %d", row[claseIdx], totalOriginal+1)
		}
	}
	if err = f.Close(); err != nil {
		log.Fatal(err)
	}

	fi, err := os.Open(entradaItems)
	if err != nil {
		log.Fatal(err)
	}
	ri := csv.NewReader(bufio.NewReaderSize(fi, 1<<20))
	ri.ReuseRecord = true
	ih, err := ri.Read()
	if err != nil {
		log.Fatal(err)
	}
	ocidIdx, statusIdx, detalleIdx := columna(ih, "main_ocid"), columna(ih, "status"), columna(ih, "statusDetails")
	itemsTotales, itemsVinculados := 0, 0
	for {
		row, e := ri.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		itemsTotales++
		est, ok := estados[strings.TrimSpace(row[ocidIdx])]
		if !ok {
			continue
		}
		itemsVinculados++
		est.items++
		if !strings.EqualFold(strings.TrimSpace(row[statusIdx]), "UNSUCCESSFUL") ||
			!strings.EqualFold(strings.TrimSpace(row[detalleIdx]), "DESIERTO") {
			est.soloDesierto = false
		}
	}
	if err = fi.Close(); err != nil {
		log.Fatal(err)
	}

	f, err = os.Open(entrada)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r = csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	header, err = r.Read()
	if err != nil {
		log.Fatal(err)
	}
	idIdx, claseIdx = columna(header, "ocid"), columna(header, "riesgo")
	fechaIdx := columna(header, "fecha_publicacion")

	fo, err := os.Create(salida)
	if err != nil {
		log.Fatal(err)
	}
	bw := bufio.NewWriterSize(fo, 1<<20)
	w := csv.NewWriter(bw)
	if err := w.Write(header); err != nil {
		log.Fatal(err)
	}

	positivos, negativosCompletos, negativosOtros, sinItems := 0, 0, 0, 0
	byYear := make(map[int][3]int)
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		categoria := 0 // 0=complete, 1=desierto puro, 2=otros desfavorables
		if row[claseIdx] == "1" {
			est := estados[strings.TrimSpace(row[idIdx])]
			if est == nil || est.items == 0 {
				sinItems++
				continue
			} // no inventar etiquetas
			if est.soloDesierto {
				categoria = 1
				row[claseIdx] = "1"
				positivos++
			} else {
				categoria = 2
				row[claseIdx] = "0"
				negativosOtros++
			}
		} else {
			row[claseIdx] = "0"
			negativosCompletos++
		}
		if len(row[fechaIdx]) >= 4 {
			if yr, e := strconv.Atoi(row[fechaIdx][:4]); e == nil {
				n := byYear[yr]
				n[categoria]++
				byYear[yr] = n
			}
		}
		if err := w.Write(row); err != nil {
			log.Fatal(err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	if err := bw.Flush(); err != nil {
		log.Fatal(err)
	}
	if err := fo.Close(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== DESIERTO PURO (1) VS TODOS LOS DEMAS (0) ===")
	fmt.Printf("Registros fuente: %d | Exportados: %d | Items recorridos: %d | Vinculados clase 1: %d\n",
		totalOriginal, positivos+negativosCompletos+negativosOtros, itemsTotales, itemsVinculados)
	fmt.Printf("Desierto puro=1: %d | Complete=0: %d | Otros desenlaces=0: %d | Sin items excluidos: %d\n",
		positivos, negativosCompletos, negativosOtros, sinItems)
	fmt.Printf("%-6s %12s %12s %15s %12s\n", "Anio", "Desierto=1", "Complete=0", "Otros=0", "Total")
	for yr := 2018; yr <= 2025; yr++ {
		n := byYear[yr]
		fmt.Printf("%-6d %12d %12d %15d %12d\n", yr, n[1], n[0], n[2], n[0]+n[1]+n[2])
	}
	fmt.Printf("Generado: %s | Tiempo: %s\n", salida, time.Since(inicio))
	fmt.Println("Advertencia: 'otros desenlaces' contiene NULO, CANCELADO y mixtos; NO significa exito.")
	fmt.Println("Advertencia: solo se evalua coherencia retrospectiva; no esta verificada la disponibilidad inicial de predictores.")
}
