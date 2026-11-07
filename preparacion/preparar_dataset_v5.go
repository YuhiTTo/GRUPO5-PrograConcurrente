// V5: dataset con variables disponibles al publicarse una licitacion.
// Lee data/main.csv y data/etiquetas_procesos.csv; no modifica versiones anteriores.
// Uso: go run preparar_dataset_v5.go
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

func pos(c []string, name string) int {
	for i, n := range c {
		if strings.TrimSpace(strings.TrimPrefix(n, "\ufeff")) == name {
			return i
		}
	}
	log.Fatalf("Falta la columna %s", name)
	return -1
}
func main() {
	ini := time.Now()
	lf, err := os.Open("data/etiquetas_procesos.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer lf.Close()
	lr := csv.NewReader(bufio.NewReaderSize(lf, 1<<20))
	lr.ReuseRecord = true
	h, err := lr.Read()
	if err != nil {
		log.Fatal(err)
	}
	oi, yi := pos(h, "main_ocid"), pos(h, "riesgo")
	etiquetas := make(map[string]string, 2500000)
	for {
		rec, e := lr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		if rec[yi] == "0" || rec[yi] == "1" {
			etiquetas[strings.TrimSpace(rec[oi])] = rec[yi]
		}
	}
	in, err := os.Open("data/main.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer in.Close()
	r := csv.NewReader(bufio.NewReaderSize(in, 1<<20))
	r.ReuseRecord = true
	h, err = r.Read()
	if err != nil {
		log.Fatal(err)
	}
	oid := pos(h, "ocid")
	mon := pos(h, "tender_value_amount_PEN")
	met := pos(h, "tender_procurementMethod")
	cat := pos(h, "tender_mainProcurementCategory")
	fecha := pos(h, "tender_datePublished")
	buyer := pos(h, "buyer_id")
	detalle := pos(h, "tender_procurementMethodDetails")
	titulo := pos(h, "tender_title")
	out, err := os.Create("data/entrenamiento_v5.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, 1<<20)
	w := csv.NewWriter(bw)
	cols := []string{"ocid", "monto_pen", "monto_disponible", "metodo", "categoria", "fecha_publicacion", "buyer_id", "metodo_detalle", "titulo", "riesgo"}
	if err = w.Write(cols); err != nil {
		log.Fatal(err)
	}
	vistos := make(map[string]struct{}, 2500000)
	var leidos, escritos, montoOK int64
	for {
		rec, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		leidos++
		id := strings.TrimSpace(rec[oid])
		y, ok := etiquetas[id]
		if !ok {
			continue
		}
		if _, ok = vistos[id]; ok {
			continue
		}
		vistos[id] = struct{}{}
		disponible, importe := "0", ""
		if x, ex := strconv.ParseFloat(strings.TrimSpace(rec[mon]), 64); ex == nil && x > 0 && !math.IsNaN(x) && !math.IsInf(x, 0) {
			disponible = "1"
			importe = strconv.FormatFloat(x, 'f', -1, 64)
			montoOK++
		}
		tituloLimpio := strings.TrimSpace(rec[titulo])
		if len(tituloLimpio) > 500 {
			tituloLimpio = string([]rune(tituloLimpio)[:min(len([]rune(tituloLimpio)), 250)])
		}
		if err = w.Write([]string{id, importe, disponible, rec[met], rec[cat], rec[fecha], rec[buyer], rec[detalle], tituloLimpio, y}); err != nil {
			log.Fatal(err)
		}
		escritos++
	}
	w.Flush()
	if err = w.Error(); err != nil {
		log.Fatal(err)
	}
	if err = bw.Flush(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Main: %d | Etiquetas: %d | Procedimientos V5: %d | Con monto: %d\n", leidos, len(etiquetas), escritos, montoOK)
	fmt.Printf("Archivo: data/entrenamiento_v5.csv | Tiempo: %v\n", time.Since(ini))
	fmt.Println("Se conservan los datasets anteriores; titulo procede de tender_title, NO del estado de adjudicacion.")
}
