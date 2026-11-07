// PC3 - Agrega campos previos al resultado sin reemplazar el dataset anterior.
// Uso: go run preparar_dataset_v3.go
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

func col(c []string, n string) int {
	for i, v := range c {
		if strings.TrimSpace(v) == n {
			return i
		}
	}
	log.Fatalf("Falta columna %s", n)
	return -1
}
func main() {
	ini := time.Now()
	f, e := os.Open("data/etiquetas_procesos.csv")
	if e != nil {
		log.Fatal(e)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	h, e := r.Read()
	if e != nil {
		log.Fatal(e)
	}
	id, cl := col(h, "main_ocid"), col(h, "riesgo")
	labels := make(map[string]string, 2500000)
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		if row[cl] == "0" || row[cl] == "1" {
			labels[strings.TrimSpace(row[id])] = row[cl]
		}
	}
	source, e := os.Open("data/main.csv")
	if e != nil {
		log.Fatal(e)
	}
	defer source.Close()
	reader := csv.NewReader(bufio.NewReaderSize(source, 1<<20))
	reader.ReuseRecord = true
	h, e = reader.Read()
	if e != nil {
		log.Fatal(e)
	}
	ocid := col(h, "ocid")
	monto := col(h, "tender_value_amount_PEN")
	metodo := col(h, "tender_procurementMethod")
	categoria := col(h, "tender_mainProcurementCategory")
	fecha := col(h, "tender_datePublished")
	buyer := col(h, "buyer_id")
	detalle := col(h, "tender_procurementMethodDetails")
	out, e := os.Create("data/entrenamiento_v3.csv")
	if e != nil {
		log.Fatal(e)
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, 1<<20)
	w := csv.NewWriter(bw)
	if e = w.Write([]string{"ocid", "monto_pen", "monto_disponible", "metodo", "categoria", "fecha_publicacion", "buyer_id", "metodo_detalle", "riesgo"}); e != nil {
		log.Fatal(e)
	}
	var total, exportados int64
	seen := make(map[string]bool, 2500000)
	for {
		row, e := reader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		total++
		key := strings.TrimSpace(row[ocid])
		label, ok := labels[key]
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		// El indicador 1 significa monto positivo numerico. Un monto ausente NO es cero.
		disponible := "0"
		valorMonto := ""
		v, errNumero := strconv.ParseFloat(strings.TrimSpace(row[monto]), 64)
		if errNumero == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 {
			disponible = "1"
			valorMonto = strconv.FormatFloat(v, 'f', -1, 64)
		}
		if e = w.Write([]string{key, valorMonto, disponible, row[metodo], row[categoria], row[fecha], row[buyer], row[detalle], label}); e != nil {
			log.Fatal(e)
		}
		exportados++
	}
	w.Flush()
	if e = w.Error(); e != nil {
		log.Fatal(e)
	}
	if e = bw.Flush(); e != nil {
		log.Fatal(e)
	}
	fmt.Printf("Main leidos: %d | Etiquetas: %d | Filas V3: %d | Tiempo: %s\n", total, len(labels), exportados, time.Since(ini))
	fmt.Println("Generado: data/entrenamiento_v3.csv (sin modificar entrenamiento_base.csv)")
}
