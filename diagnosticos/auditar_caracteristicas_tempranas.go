// Auditoria exploratoria de cobertura de variables candidatas por anio y clase.
// NO verifica disponibilidad al momento de la convocatoria: para eso se necesitan
// releases/snapshots historicos de OECE, no solo el CSV consolidado.
// Ejecucion desde raiz: go run auditar_caracteristicas_tempranas.go
// Solo lectura: data/main.csv, data/etiquetas_procesos.csv.
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Campo struct {
	nombre    string
	columna   int
	cobertura [2]int64
	tipos     map[string]int64
}
type Grupo struct {
	total  [2]int64
	campos []Campo
}

func indice(c []string, s string) int {
	for i, x := range c {
		if strings.TrimPrefix(strings.TrimSpace(x), "\ufeff") == s {
			return i
		}
	}
	return -1
}
func leer(path string) (*os.File, *csv.Reader) {
	f, e := os.Open(path)
	if e != nil {
		log.Fatal(e)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	return f, r
}
func noVacio(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && strings.ToUpper(s) != "NULL" && strings.ToUpper(s) != "N/A"
}
func valor(s string) bool {
	s = strings.TrimSpace(s)
	if !noVacio(s) {
		return false
	}
	v, e := strconv.ParseFloat(s, 64)
	return e == nil && v > 0
}
func validoCampo(s, nombre string) bool {
	if nombre == "tender_value_amount" || nombre == "tender_value_amount_PEN" || nombre == "planning_budget_amount_amount" || strings.Contains(nombre, "durationInDays") || nombre == "tender_numberOfTenderers" {
		return valor(s)
	}
	return noVacio(s)
}
func cobertura(x int64, n int64) string {
	if n == 0 {
		return "N/A"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(x)/float64(n))
}
func main() {
	start := time.Now()
	const etiquetas = "data/etiquetas_procesos.csv"
	ef, er := leer(etiquetas)
	defer ef.Close()
	h, e := er.Read()
	if e != nil {
		log.Fatal(e)
	}
	id, y := indice(h, "main_ocid"), indice(h, "riesgo")
	if id < 0 || y < 0 {
		log.Fatal("Faltan columnas de etiquetas")
	}
	clases := make(map[string]uint8, 2500000)
	for {
		r, e := er.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		if r[y] == "0" {
			clases[strings.TrimSpace(r[id])] = 0
		} else if r[y] == "1" {
			clases[strings.TrimSpace(r[id])] = 1
		}
	}
	mf, mr := leer("data/main.csv")
	defer mf.Close()
	h, e = mr.Read()
	if e != nil {
		log.Fatal(e)
	}
	oi, fi := indice(h, "ocid"), indice(h, "tender_datePublished")
	if oi < 0 || fi < 0 {
		log.Fatal("Faltan columnas base")
	}
	// Aspirantes: revisar si su valor estaba publicado antes del resultado.
	names := []string{
		"tender_tenderPeriod_startDate", "tender_tenderPeriod_endDate", "tender_tenderPeriod_durationInDays",
		"planning_budget_amount_amount", "planning_budget_project", "planning_budget_description",
		"tender_enquiryPeriod_startDate", "tender_enquiryPeriod_durationInDays",
		"tender_description", "tender_awardCriteria", "tender_awardCriteriaDetails",
		"tender_numberOfTenderers", // POST-CONVOCATORIA: no usar para prediccion al publicar.
	}
	available := []Campo{}
	for _, n := range names {
		i := indice(h, n)
		if i >= 0 {
			available = append(available, Campo{nombre: n, columna: i})
		} else {
			fmt.Printf("AVISO: No existe columna %s\n", n)
		}
	}
	years := map[int]*Grupo{}
	vistos := make(map[string]bool, 2500000)
	var used, wrongDate, missingLabel int64
	for {
		r, e := mr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		ocid := strings.TrimSpace(r[oi])
		cl, ok := clases[ocid]
		if !ok {
			missingLabel++
			continue
		}
		if vistos[ocid] {
			continue
		}
		vistos[ocid] = true
		fecha := strings.TrimSpace(r[fi])
		if len(fecha) < 4 {
			wrongDate++
			continue
		}
		a, err := strconv.Atoi(fecha[:4])
		if err != nil || a < 2018 || a > 2025 {
			if err != nil {
				wrongDate++
			}
			continue
		}
		g := years[a]
		if g == nil {
			g = &Grupo{campos: make([]Campo, len(available))}
			copy(g.campos, available)
			years[a] = g
		}
		g.total[cl]++
		used++
		for i := range g.campos {
			c := &g.campos[i]
			if validoCampo(r[c.columna], c.nombre) {
				c.cobertura[cl]++
			}
		}
	}
	fmt.Printf("=== COBERTURA DE CAMPOS CANDIDATOS POR ANIO Y CLASE ===\nProcesos 2018-2025: %d | Sin etiqueta: %d | Fecha invalida: %d\n", used, missingLabel, wrongDate)
	keys := make([]int, 0, len(years))
	for k := range years {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, a := range keys {
		g := years[a]
		fmt.Printf("\nANIO %d | clase 0=%d | clase 1=%d | total=%d\n", a, g.total[0], g.total[1], g.total[0]+g.total[1])
		fmt.Printf("%-42s %12s %12s %12s\n", "CAMPO", "C0 PRES.", "C1 PRES.", "TOTAL PRES.")
		for _, c := range g.campos {
			fmt.Printf("%-42s %12s %12s %12s\n", c.nombre, cobertura(c.cobertura[0], g.total[0]), cobertura(c.cobertura[1], g.total[1]), cobertura(c.cobertura[0]+c.cobertura[1], g.total[0]+g.total[1]))
		}
	}
	fmt.Println("\nADVERTENCIAS: Cobertura NO prueba disponibilidad antes del desenlace. tender_numberOfTenderers es posterior a la presentacion de ofertas; NO usar para predecir en fecha de convocatoria. Datos compilados pueden contener actualizaciones posteriores.")
	fmt.Println("Tiempo:", time.Since(start))
}
