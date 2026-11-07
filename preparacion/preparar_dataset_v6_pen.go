// V6 - Reconstruccion prudente del importe y cohorte con valores en PEN.
// Lee data/main.csv y data/etiquetas_procesos.csv.
// Crea data/entrenamiento_v6_pen.csv sin sobrescribir datasets anteriores.
// Solo incluye importes positivos expresados en PEN:
//  1. tender_value_amount_PEN, cuando existe;
//  2. tender_value_amount, SOLO si tender_value_currency == PEN.
//
// No mezcla presupuestos ni montos en monedas extranjeras sin conversion.
// La disponibilidad es CONSTANTE (=1) en el dataset de salida.
// IMPORTANTE: la tabla aplanada es un estado compilado; NO prueba que
// los importes estuvieran disponibles en la fecha de publicacion.
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func columna(h []string, name string) int {
	for i, s := range h {
		if strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")) == name {
			return i
		}
	}
	log.Fatalf("Falta columna %q", name)
	return -1
}
func montoPositivo(s string) (float64, bool) {
	v, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v, e == nil && v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}
func abrirCSV(path string) (*os.File, *csv.Reader) {
	f, e := os.Open(path)
	if e != nil {
		log.Fatal(e)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	return f, r
}

type anioCuenta struct{ total, pen, recuperado, excluido int64 }

func main() {
	start := time.Now()
	lf, lr := abrirCSV("data/etiquetas_procesos.csv")
	defer lf.Close()
	h, e := lr.Read()
	if e != nil {
		log.Fatal(e)
	}
	ixID, ixY := columna(h, "main_ocid"), columna(h, "riesgo")
	labels := make(map[string]string, 2500000)
	for {
		row, e := lr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		if row[ixY] == "0" || row[ixY] == "1" {
			labels[strings.TrimSpace(row[ixID])] = row[ixY]
		}
	}
	mf, mr := abrirCSV("data/main.csv")
	defer mf.Close()
	h, e = mr.Read()
	if e != nil {
		log.Fatal(e)
	}
	oi := columna(h, "ocid")
	pen := columna(h, "tender_value_amount_PEN")
	orig := columna(h, "tender_value_amount")
	moneda := columna(h, "tender_value_currency")
	metodo := columna(h, "tender_procurementMethod")
	cat := columna(h, "tender_mainProcurementCategory")
	fecha := columna(h, "tender_datePublished")
	buyer := columna(h, "buyer_id")
	detalle := columna(h, "tender_procurementMethodDetails")
	titulo := columna(h, "tender_title")
	if e := os.MkdirAll("data", 0755); e != nil {
		log.Fatal(e)
	}
	outPath := "data/entrenamiento_v6_pen.csv"
	out, e := os.Create(outPath)
	if e != nil {
		log.Fatal(e)
	}
	defer out.Close()
	b := bufio.NewWriterSize(out, 1<<20)
	w := csv.NewWriter(b)
	if e := w.Write([]string{"ocid", "monto_pen", "monto_disponible", "metodo", "categoria", "fecha_publicacion", "buyer_id", "metodo_detalle", "titulo", "riesgo", "origen_monto"}); e != nil {
		log.Fatal(e)
	}
	vistos := make(map[string]struct{}, len(labels))
	var leidos, escritos, penOK, fallback, rechazados, sinEtiqueta, duplicados int64
	years := make(map[string]*anioCuenta)
	for {
		row, e := mr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		leidos++
		id := strings.TrimSpace(row[oi])
		y, ok := labels[id]
		if !ok || id == "" {
			sinEtiqueta++
			continue
		}
		if _, ya := vistos[id]; ya {
			duplicados++
			continue
		}
		vistos[id] = struct{}{}
		year := "SIN_FECHA"
		if len(row[fecha]) >= 4 {
			year = row[fecha][:4]
		}
		a := years[year]
		if a == nil {
			a = &anioCuenta{}
			years[year] = a
		}
		a.total++
		value, valid := montoPositivo(row[pen])
		fuente := "PEN_CONVERTIDO"
		if valid {
			penOK++
			a.pen++
		} else {
			candidate, ok := montoPositivo(row[orig])
			currency := strings.ToUpper(strings.TrimSpace(row[moneda]))
			if ok && currency == "PEN" {
				value = candidate
				valid = true
				fuente = "ORIGINAL_PEN"
				fallback++
				a.recuperado++
			}
		}
		if !valid {
			rechazados++
			a.excluido++
			continue
		}
		t := strings.TrimSpace(row[titulo])
		if len([]rune(t)) > 250 {
			t = string([]rune(t)[:250])
		}
		rec := []string{id, strconv.FormatFloat(value, 'f', -1, 64), "1", row[metodo], row[cat], row[fecha], row[buyer], row[detalle], t, y, fuente}
		if e := w.Write(rec); e != nil {
			log.Fatal(e)
		}
		escritos++
	}
	w.Flush()
	if e := w.Error(); e != nil {
		log.Fatal(e)
	}
	if e := b.Flush(); e != nil {
		log.Fatal(e)
	}
	fmt.Println("=== DATASET V6: SOLO IMPORTES VALIDOS EN SOLES ===")
	fmt.Printf("main: %d | etiquetas: %d | sin etiqueta: %d | duplicados: %d\n", leidos, len(labels), sinEtiqueta, duplicados)
	fmt.Printf("PEN convertido: %d | recuperado desde original PEN: %d | excluidos: %d | exportados: %d\n", penOK, fallback, rechazados, escritos)
	keys := make([]string, 0, len(years))
	for k := range years {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\n%-8s %11s %11s %12s %11s\n", "ANIO", "TOTAL", "PEN OK", "RECUPERADOS", "EXCLUIDOS")
	for _, k := range keys {
		if k < "2018" || k > "2025" {
			continue
		}
		a := years[k]
		fmt.Printf("%-8s %11d %11d %12d %11d\n", k, a.total, a.pen, a.recuperado, a.excluido)
	}
	fmt.Printf("\nArchivo: %s | tiempo: %s\n", outPath, time.Since(start))
	fmt.Println("Advertencia: esta cohorte excluye registros sin monto PEN comparable; NO demuestra ausencia de fuga temporal.")
}
