// Auditoria reproducible de diez OCID de SEACE/OCDS.
// Uso: go run auditar_10_ocids.go
// Lee data/main.csv y data/tender_items.csv; crea data/auditoria_10_ocids.csv.
// IMPORTANTE: main.csv contiene registros consolidados, NO primeras releases.
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

type caso struct {
	ocid, clase string
	campos      map[string]string
	conteos     map[string]int
	detalles    map[string]int
	items       int
}

var muestras = []struct{ ocid, clase string }{
	{"ocds-dgv273-seacev3-2023-1002998632-1", "0"},
	{"ocds-dgv273-seacev3-2023-1003005396-1", "0"},
	{"ocds-dgv273-seacev3-2023-1025-23", "0"},
	{"ocds-dgv273-seacev3-2023-1158-19", "0"},
	{"ocds-dgv273-seacev3-2023-1158-3", "0"},
	{"ocds-dgv273-seacev3-2023-1254-6", "1"},
	{"ocds-dgv273-seacev3-2023-20-3", "1"},
	{"ocds-dgv273-seacev3-2023-206330-7", "1"},
	{"ocds-dgv273-seacev3-2023-2347-51", "1"},
	{"ocds-dgv273-seacev3-2023-2406-456", "1"},
}

var camposMain = []string{
	"tender_datePublished", "publishedDate", "date", "tag", "buyer_name",
	"tender_title", "tender_procurementMethod", "tender_procurementMethodDetails",
	"tender_value_amount_PEN", "tender_value_amount", "tender_value_currency",
	"planning_budget_amount_amount", "tender_tenderPeriod_startDate", "tender_tenderPeriod_endDate",
	"tender_enquiryPeriod_startDate", "tender_enquiryPeriod_endDate",
}

func indices(h []string) map[string]int {
	m := map[string]int{}
	for i, n := range h {
		m[strings.TrimSpace(strings.TrimPrefix(n, "\ufeff"))] = i
	}
	return m
}
func obligatorio(m map[string]int, n string) int {
	i, ok := m[n]
	if !ok {
		log.Fatalf("Falta columna %q", n)
	}
	return i
}
func limpio(s string) string { return strings.TrimSpace(s) }
func escoger(m map[string]int, row []string, n string) string {
	i, ok := m[n]
	if !ok || i >= len(row) {
		return ""
	}
	return limpio(row[i])
}
func resumen(m map[string]int) string {
	ks := make([]string, 0, len(m))
	for s := range m {
		ks = append(ks, s)
	}
	sort.Strings(ks)
	out := make([]string, 0, len(ks))
	for _, s := range ks {
		k := s
		if k == "" {
			k = "(VACIO)"
		}
		out = append(out, fmt.Sprintf("%s:%d", k, m[s]))
	}
	return strings.Join(out, " | ")
}

func main() {
	inicio := time.Now()
	casos := map[string]*caso{}
	for _, x := range muestras {
		casos[x.ocid] = &caso{ocid: x.ocid, clase: x.clase, campos: map[string]string{}, conteos: map[string]int{}, detalles: map[string]int{}}
	}
	f, e := os.Open("data/main.csv")
	if e != nil {
		log.Fatal(e)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	h, e := r.Read()
	if e != nil {
		log.Fatal(e)
	}
	idx := indices(h)
	iocid := obligatorio(idx, "ocid")
	var nMain int
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		nMain++
		if c := casos[limpio(row[iocid])]; c != nil {
			for _, n := range camposMain {
				c.campos[n] = escoger(idx, row, n)
			}
		}
	}
	if e = f.Close(); e != nil {
		log.Fatal(e)
	}
	f, e = os.Open("data/tender_items.csv")
	if e != nil {
		log.Fatal(e)
	}
	r = csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	h, e = r.Read()
	if e != nil {
		log.Fatal(e)
	}
	idx = indices(h)
	iocid = obligatorio(idx, "main_ocid")
	istatus := obligatorio(idx, "status")
	iDetalle := obligatorio(idx, "statusDetails")
	var nItems int
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		nItems++
		if c := casos[limpio(row[iocid])]; c != nil {
			c.items++
			c.conteos[limpio(row[istatus])]++
			c.detalles[limpio(row[iDetalle])]++
		}
	}
	if e = f.Close(); e != nil {
		log.Fatal(e)
	}
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatal(err)
	}
	out, e := os.Create("data/auditoria_10_ocids.csv")
	if e != nil {
		log.Fatal(e)
	}
	defer out.Close()
	w := csv.NewWriter(out)
	header := []string{"ocid", "etiqueta_derivada", "items", "status_items", "statusDetails_items"}
	header = append(header, camposMain...)
	header = append(header, "observacion_temporal")
	if err := w.Write(header); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("=== AUDITORIA 10 OCID (DATOS CONSOLIDADOS) ===\nmain.csv: %d filas | tender_items.csv: %d filas\n\n", nMain, nItems)
	for _, x := range muestras {
		c := casos[x.ocid]
		row := []string{c.ocid, c.clase, fmt.Sprint(c.items), resumen(c.conteos), resumen(c.detalles)}
		for _, n := range camposMain {
			row = append(row, c.campos[n])
		}
		row = append(row, "Campos del CSV consolidado; fecha inicial y valores de primera release NO verificados")
		if err := w.Write(row); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Clase %s | %s\n", c.clase, c.ocid)
		fmt.Printf("  Publicacion consignada: %s | items=%d\n", c.campos["tender_datePublished"], c.items)
		fmt.Printf("  Metodo: %s | detalle: %s\n", c.campos["tender_procurementMethod"], c.campos["tender_procurementMethodDetails"])
		fmt.Printf("  Monto PEN: %q | original: %q %q | presupuesto: %q\n", c.campos["tender_value_amount_PEN"], c.campos["tender_value_amount"], c.campos["tender_value_currency"], c.campos["planning_budget_amount_amount"])
		fmt.Printf("  Estado(s): %s\n", resumen(c.conteos))
		fmt.Printf("  Detalle(s): %s\n\n", resumen(c.detalles))
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Archivo generado: data/auditoria_10_ocids.csv")
	fmt.Println("ADVERTENCIA: este reporte NO reconstruye la primera convocatoria ni demuestra disponibilidad temporal de los campos.")
	fmt.Printf("Tiempo: %s\n", time.Since(inicio))
}
