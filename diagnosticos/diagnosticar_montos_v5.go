// PC3 - Diagnostico de montos por periodo y por clase (no modifica archivos).
// Uso: go run diagnosticar_montos_v5.go
// ATENCION: correlacion de faltantes con resultados NO implica prediccion anticipada.
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

type Resumen struct{ Total, Pos int64 }

func (r *Resumen) add(pos bool) {
	r.Total++
	if pos {
		r.Pos++
	}
}
func pct(num, den int64) float64 {
	if den == 0 {
		return 0
	}
	return 100 * float64(num) / float64(den)
}
func col(c []string, n string) int {
	for i, s := range c {
		if strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")) == n {
			return i
		}
	}
	log.Fatalf("Falta columna %q", n)
	return -1
}

type Anual struct{ Presente, Ausente Resumen }

func banda(monto float64) int {
	switch {
	case monto <= 10000:
		return 0
	case monto <= 100000:
		return 1
	case monto <= 1000000:
		return 2
	case monto <= 10000000:
		return 3
	default:
		return 4
	}
}
func main() {
	start := time.Now()
	f, e := os.Open("data/entrenamiento_v5.csv")
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
	iy, im, id, ifec := col(h, "riesgo"), col(h, "monto_pen"), col(h, "monto_disponible"), col(h, "fecha_publicacion")
	byYear := map[int]*Anual{}
	byBand := map[string][5]Resumen{}
	var bad int64
	for {
		rec, er := r.Read()
		if er == io.EOF {
			break
		}
		if er != nil {
			log.Fatal(er)
		}
		fecha := strings.TrimSpace(rec[ifec])
		if len(fecha) < 4 || (rec[iy] != "0" && rec[iy] != "1") {
			bad++
			continue
		}
		year, er := strconv.Atoi(fecha[:4])
		if er != nil || year < 2018 || year > 2025 {
			continue
		}
		y := rec[iy] == "1"
		a := byYear[year]
		if a == nil {
			a = &Anual{}
			byYear[year] = a
		}
		monto, er := strconv.ParseFloat(strings.TrimSpace(rec[im]), 64)
		disponible := rec[id] == "1" && er == nil && monto > 0 && !math.IsNaN(monto) && !math.IsInf(monto, 0)
		if disponible {
			a.Presente.add(y)
		} else {
			a.Ausente.add(y)
		}
		if disponible {
			grupo := "2018-2022"
			if year >= 2023 {
				grupo = strconv.Itoa(year)
			}
			datos := byBand[grupo]
			b := banda(monto)
			datos[b].add(y)
			byBand[grupo] = datos
		}
	}
	fmt.Println("=== AUDITORIA DE MONTO V5 POR ANIO ===")
	fmt.Printf("%-9s %10s %12s %13s %15s %15s\n", "ANIO", "TOTAL", "% MONTO OK", "% CL1 TOTAL", "% CL1 CON MONTO", "% CL1 SIN MONTO")
	years := make([]int, 0, len(byYear))
	for k := range byYear {
		years = append(years, k)
	}
	sort.Ints(years)
	for _, year := range years {
		a := byYear[year]
		t := a.Presente.Total + a.Ausente.Total
		p := a.Presente.Pos + a.Ausente.Pos
		fmt.Printf("%-9d %10d %11.2f%% %12.2f%% %14.2f%% %14.2f%%\n", year, t, pct(a.Presente.Total, t), pct(p, t), pct(a.Presente.Pos, a.Presente.Total), pct(a.Ausente.Pos, a.Ausente.Total))
	}
	fmt.Println("\n=== POR BANDAS DE IMPORTE (SOLO MONTOS POSITIVOS) ===")
	bandas := []string{"(0, 10 mil]", "(10 mil, 100 mil]", "(100 mil, 1 millon]", "(1 millon, 10 millones]", "> 10 millones"}
	for _, periodo := range []string{"2018-2022", "2023", "2024", "2025"} {
		fmt.Printf("\nPERIODO %s\n%-25s %12s %12s\n", periodo, "BANDA PEN", "REGISTROS", "% CLASE 1")
		d := byBand[periodo]
		for i, r := range d {
			fmt.Printf("%-25s %12d %11.2f%%\n", bandas[i], r.Total, pct(r.Pos, r.Total))
		}
	}
	fmt.Printf("\nRegistros invalidos detectados: %d | Tiempo: %v\n", bad, time.Since(start))
	fmt.Println("Advertencia: correlacion con ausencias no demuestra que el monto fuera conocido antes del desenlace.")
}
