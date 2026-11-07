// PC3 - Auditoria de procedencia del importe previo a nuevas mejoras del modelo.
// Solo lectura: data/main.csv + data/etiquetas_procesos.csv.
// No crea ni modifica datasets/modelos. Uso: go run auditar_fuentes_monto.go
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

type conteo struct{ total, desfavorables int64 }

func (c *conteo) inc(y uint8) {
	c.total++
	if y == 1 {
		c.desfavorables++
	}
}
func porcentaje(x, total int64) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(x) / float64(total)
}
func tasa(c conteo) float64 { return porcentaje(c.desfavorables, c.total) }
func indiceCabecera(c []string, nombre string) int {
	for i, s := range c {
		if strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")) == nombre {
			return i
		}
	}
	log.Fatalf("No existe la columna %s", nombre)
	return -1
}
func importeValido(s string) bool {
	v, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return e == nil && v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}
func cargarEtiquetas() map[string]uint8 {
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
	id, y := indiceCabecera(h, "main_ocid"), indiceCabecera(h, "riesgo")
	etiquetas := make(map[string]uint8, 2500000)
	for {
		fila, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		var clase uint8
		switch strings.TrimSpace(fila[y]) {
		case "0":
			clase = 0
		case "1":
			clase = 1
		default:
			continue
		}
		etiquetas[strings.TrimSpace(fila[id])] = clase
	}
	return etiquetas
}
func main() {
	inicio := time.Now()
	labels := cargarEtiquetas()
	f, e := os.Open("data/main.csv")
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
	ocid := indiceCabecera(h, "ocid")
	pen := indiceCabecera(h, "tender_value_amount_PEN")
	orig := indiceCabecera(h, "tender_value_amount")
	moneda := indiceCabecera(h, "tender_value_currency")
	presupuesto := indiceCabecera(h, "planning_budget_amount_amount")
	fecha := indiceCabecera(h, "tender_datePublished")
	estados := map[string]*conteo{}
	type anioStats struct{ total, pen, original, presup, ninguno, cl1SinPen, sinPen int64 }
	anios := map[string]*anioStats{}
	vistos := make(map[string]struct{}, 2500000)
	var totalMain, sinEtiqueta, duplicados, sinFecha int64
	for {
		fila, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		totalMain++
		id := strings.TrimSpace(fila[ocid])
		y, ok := labels[id]
		if !ok {
			sinEtiqueta++
			continue
		}
		if _, ya := vistos[id]; ya {
			duplicados++
			continue
		}
		vistos[id] = struct{}{}
		year := "OTROS"
		s := strings.TrimSpace(fila[fecha])
		if len(s) >= 4 {
			if x, err := strconv.Atoi(s[:4]); err == nil && x >= 1990 && x <= 2030 {
				year = strconv.Itoa(x)
			}
		}
		if year == "OTROS" {
			sinFecha++
		}
		ps, os_, bs := importeValido(fila[pen]), importeValido(fila[orig]), importeValido(fila[presupuesto])
		curr := strings.ToUpper(strings.TrimSpace(fila[moneda]))
		clase := ""
		switch {
		case ps:
			clase = "PEN_VALIDO"
		case os_ && curr == "PEN":
			clase = "PEN_VACIO_ORIGINAL_PEN"
		case os_ && curr != "PEN":
			clase = "PEN_VACIO_ORIGINAL_OTRA_MONEDA"
		case bs:
			clase = "SOLO_PRESUPUESTO_VALIDO"
		default:
			clase = "NINGUN_IMPORTE_VALIDO"
		}
		c := estados[clase]
		if c == nil {
			c = &conteo{}
			estados[clase] = c
		}
		c.inc(y)
		a := anios[year]
		if a == nil {
			a = &anioStats{}
			anios[year] = a
		}
		a.total++
		if ps {
			a.pen++
		} else {
			a.sinPen++
			if y == 1 {
				a.cl1SinPen++
			}
		}
		if os_ {
			a.original++
		}
		if bs {
			a.presup++
		}
		if !ps && !os_ && !bs {
			a.ninguno++
		}
	}
	fmt.Println("=== AUDITORIA DE FUENTES DE MONTO (SOLO LECTURA) ===")
	fmt.Printf("main.csv leidos: %d | Etiquetas: %d | Con etiqueta y OCID unico: %d | Sin etiqueta: %d | Duplicados: %d | Fecha no reconocida: %d\n", totalMain, len(labels), len(vistos), sinEtiqueta, duplicados, sinFecha)
	fmt.Println("\n=== SI NO EXISTE tender_value_amount_PEN, ¿HAY OTRA FUENTE? ===")
	fmt.Printf("%-36s %11s %11s %12s\n", "CASO", "REGISTROS", "CLASE 1", "% CLASE 1")
	order := []string{"PEN_VALIDO", "PEN_VACIO_ORIGINAL_PEN", "PEN_VACIO_ORIGINAL_OTRA_MONEDA", "SOLO_PRESUPUESTO_VALIDO", "NINGUN_IMPORTE_VALIDO"}
	for _, k := range order {
		v := estados[k]
		if v == nil {
			v = &conteo{}
		}
		fmt.Printf("%-36s %11d %11d %11.2f%%\n", k, v.total, v.desfavorables, tasa(*v))
	}
	fmt.Println("\n=== DISPONIBILIDAD DE CAMPOS POR ANIO ===")
	fmt.Printf("%-7s %10s %10s %10s %10s %10s %12s\n", "ANIO", "TOTAL", "% PEN OK", "% ORIG OK", "% PRES OK", "% NINGUNO", "% CL1 SIN PEN")
	years := make([]string, 0, len(anios))
	for k := range anios {
		years = append(years, k)
	}
	sort.Strings(years)
	for _, k := range years {
		if k < "2018" || k > "2025" {
			continue
		}
		v := anios[k]
		fmt.Printf("%-7s %10d %9.2f%% %9.2f%% %9.2f%% %9.2f%% %11.2f%%\n", k, v.total, porcentaje(v.pen, v.total), porcentaje(v.original, v.total), porcentaje(v.presup, v.total), porcentaje(v.ninguno, v.total), porcentaje(v.cl1SinPen, v.sinPen))
	}
	fmt.Printf("\nTiempo: %v\n", time.Since(inicio))
	fmt.Println("IMPORTANTE: importe original y presupuesto NO son equivalentes ni intercambiables sin validacion de definiciones y moneda.")
	fmt.Println("El origen temporal de un campo no se puede verificar con este archivo aplanado: requiere historial/snapshots de publicaciones.")
}
