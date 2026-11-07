// Auditoria muestral de metadatos temporales en el CSV consolidado.
// Solo lee los CSV locales. NO verifica si un campo estuvo disponible
// durante la convocatoria: para eso hacen falta registros historicos.
// Ejecutar desde la raiz del proyecto:
// go run .\diagnosticos\auditar_temporalidad_desiertos.go
package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	original = "data/entrenamiento_controlado.csv"
	focal    = "data/entrenamiento_desierto_vs_resto.csv"
	fuente   = "data/main.csv"
	salida   = "data/auditoria_temporalidad_desiertos.csv"
	porGrupo = 12
)

type Muestra struct {
	id     string
	anio   int
	grupo  string
	riesgo string
	score  uint64
}

func ix(header []string, clave string) int {
	for i, c := range header {
		if strings.TrimPrefix(strings.TrimSpace(c), "\ufeff") == clave {
			return i
		}
	}
	log.Fatalf("Falta columna %q", clave)
	return -1
}
func fCSV(ruta string) (*os.File, *csv.Reader) {
	f, e := os.Open(ruta)
	if e != nil {
		log.Fatal(e)
	}
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	return f, r
}
func ano(s string) int {
	if len(s) < 4 {
		return 0
	}
	n, _ := strconv.Atoi(s[:4])
	return n
}
func hash(s string) uint64 { h := fnv.New64a(); _, _ = h.Write([]byte(s)); return h.Sum64() }
func agregar(bins map[string][]Muestra, s Muestra) {
	k := fmt.Sprintf("%d_%s", s.anio, s.grupo)
	z := append(bins[k], s)
	sort.Slice(z, func(i, j int) bool {
		if z[i].score == z[j].score {
			return z[i].id < z[j].id
		}
		return z[i].score < z[j].score
	})
	if len(z) > porGrupo {
		z = z[:porGrupo]
	}
	bins[k] = z
}
func fecha(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000-07:00", "2006-01-02T15:04:05", "2006-01-02"} {
		t, e := time.Parse(layout, s)
		if e == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
func acortar(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	rr := []rune(s)
	if len(rr) > max {
		return string(rr[:max]) + "..."
	}
	return s
}
func main() {
	inicio := time.Now()
	// Etiqueta general: distinguir completados (clase 0) de otros
	// resultados adversos que ahora se recodificaron a 0.
	orig := make(map[string]string, 150000)
	f, r := fCSV(original)
	h, e := r.Read()
	if e != nil {
		log.Fatal(e)
	}
	iID, iY, iFecha := ix(h, "ocid"), ix(h, "riesgo"), ix(h, "fecha_publicacion")
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		y := ano(row[iFecha])
		if y == 2023 || y == 2024 {
			orig[strings.TrimSpace(row[iID])] = row[iY]
		}
	}
	_ = f.Close()
	bins := make(map[string][]Muestra)
	f, r = fCSV(focal)
	h, e = r.Read()
	if e != nil {
		log.Fatal(e)
	}
	iID, iY, iFecha = ix(h, "ocid"), ix(h, "riesgo"), ix(h, "fecha_publicacion")
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		y := ano(row[iFecha])
		if y != 2023 && y != 2024 {
			continue
		}
		id := strings.TrimSpace(row[iID])
		originalY, ok := orig[id]
		if !ok {
			log.Fatalf("OCID %q no encontrado en original", id)
		}
		grupo := "OTRO_RESULTADO"
		if row[iY] == "1" {
			grupo = "DESIERTO"
		} else if originalY == "0" {
			grupo = "COMPLETADO"
		}
		agregar(bins, Muestra{id: id, anio: y, grupo: grupo, riesgo: row[iY], score: hash(id)})
	}
	_ = f.Close()
	escogidos := make(map[string]Muestra)
	for _, s := range bins {
		for _, z := range s {
			escogidos[z.id] = z
		}
	}

	f, r = fCSV(fuente)
	defer f.Close()
	h, e = r.Read()
	if e != nil {
		log.Fatal(e)
	}
	iID = ix(h, "ocid")
	nombres := []string{"tag", "date", "publishedDate", "tender_datePublished", "tender_tenderPeriod_startDate", "tender_tenderPeriod_endDate", "tender_title", "tender_description", "tender_procurementMethod", "tender_procurementMethodDetails"}
	inds := make(map[string]int)
	for _, n := range nombres {
		inds[n] = ix(h, n)
	}
	extraidos := make(map[string][]string)
	for {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		id := strings.TrimSpace(row[iID])
		if _, ok := escogidos[id]; !ok {
			continue
		}
		if _, ya := extraidos[id]; ya {
			continue
		}
		vals := make([]string, len(nombres))
		for j, n := range nombres {
			vals[j] = row[inds[n]]
		}
		extraidos[id] = vals
		if len(extraidos) == len(escogidos) {
			break
		}
	}
	out, e := os.Create(salida)
	if e != nil {
		log.Fatal(e)
	}
	bw := bufio.NewWriterSize(out, 1<<18)
	w := csv.NewWriter(bw)
	cols := []string{"ocid", "anio", "grupo", "desierto_1", "tag", "date", "publishedDate", "tender_datePublished", "tender_tenderPeriod_startDate", "tender_tenderPeriod_endDate", "dias_date_menos_convocatoria", "fecha_ultima_date_30d_despues", "titulo_fuente", "descripcion_fuente", "metodo", "detalle"}
	if e = w.Write(cols); e != nil {
		log.Fatal(e)
	}
	keys := make([]Muestra, 0, len(escogidos))
	for _, v := range escogidos {
		keys = append(keys, v)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].anio != keys[j].anio {
			return keys[i].anio < keys[j].anio
		}
		if keys[i].grupo != keys[j].grupo {
			return keys[i].grupo < keys[j].grupo
		}
		return keys[i].id < keys[j].id
	})
	var posterior, comparables, faltantes int
	for _, s := range keys {
		vals, ok := extraidos[s.id]
		if !ok {
			faltantes++
			continue
		}
		x := make(map[string]string)
		for j, n := range nombres {
			x[n] = vals[j]
		}
		dias, marca := "", "INDETERMINADO"
		t0, ok0 := fecha(x["tender_datePublished"])
		t1, ok1 := fecha(x["date"])
		if ok0 && ok1 {
			n := int(t1.Sub(t0).Hours() / 24)
			dias = strconv.Itoa(n)
			comparables++
			marca = "NO"
			if n > 30 {
				posterior++
				marca = "SI"
			}
		}
		row := []string{s.id, strconv.Itoa(s.anio), s.grupo, s.riesgo, x["tag"], x["date"], x["publishedDate"], x["tender_datePublished"], x["tender_tenderPeriod_startDate"], x["tender_tenderPeriod_endDate"], dias, marca, acortar(x["tender_title"], 250), acortar(x["tender_description"], 500), x["tender_procurementMethod"], x["tender_procurementMethodDetails"]}
		if e = w.Write(row); e != nil {
			log.Fatal(e)
		}
	}
	w.Flush()
	if e = w.Error(); e != nil {
		log.Fatal(e)
	}
	if e = bw.Flush(); e != nil {
		log.Fatal(e)
	}
	if e = out.Close(); e != nil {
		log.Fatal(e)
	}
	fmt.Println("=== AUDITORIA MUESTRAL DE METADATOS TEMPORALES (NO PRUEBA DISPONIBILIDAD HISTORICA) ===")
	fmt.Printf("Muestra: %d solicitados, %d encontrados, %d ausentes en main.csv\n", len(escogidos), len(extraidos), faltantes)
	for _, yr := range []int{2023, 2024} {
		for _, g := range []string{"DESIERTO", "COMPLETADO", "OTRO_RESULTADO"} {
			fmt.Printf("%d %-16s %d ejemplos\n", yr, g, len(bins[fmt.Sprintf("%d_%s", yr, g)]))
		}
	}
	fmt.Printf("Fechas date y tender_datePublished comparables: %d; date >30 dias despues: %d\n", comparables, posterior)
	fmt.Printf("Reporte: %s | Duracion: %v\n", salida, time.Since(inicio))
	fmt.Println("IMPORTANTE: 'date' posterior NO demuestra que la descripcion se actualizo en ese momento. Tampoco 'date' anterior demuestra que el texto era original.")
	fmt.Println("Para validar prediccion al publicar la convocatoria se requieren registros/revisiones historicas o confirmacion de trazabilidad de origen.")
}
