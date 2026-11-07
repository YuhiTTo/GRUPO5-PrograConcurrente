// PC3 - Preparacion controlada de caracteristicas candidatas para convocatoria.
// Mismos CSV originales, sin importes, presupuesto, numero de postores ni estados como predictores.
// IMPORTANTE: el archivo fuente esta consolidado; NO demuestra disponibilidad temporal original.
// Ejecutar desde la raiz: go run .\preparacion\preparar_dataset_controlado.go
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

func indice(cab []string, nombre string) int {
	for i, x := range cab {
		if strings.TrimSpace(strings.TrimPrefix(x, "\ufeff")) == nombre {
			return i
		}
	}
	log.Fatalf("Falta columna %q", nombre)
	return -1
}
func textoCorto(s string, maxRunas int) string {
	s = strings.TrimSpace(s)
	rr := []rune(s)
	if len(rr) > maxRunas {
		rr = rr[:maxRunas]
	}
	return string(rr)
}

// Si aparece un posible estado final dentro del texto, se excluye TODO ese campo.
// Regla conservadora: no garantiza eliminar todas las pistas de etapas posteriores.
func sospechoso(s string) bool {
	t := strings.ToUpper(s)
	for _, x := range []string{"DESIERTO", "CANCELAD", "ANULAD", "DECLARADO NULO", "NULIDAD", "ADJUDICAD", "CONTRATADO", "BUENA PRO CONSENTIDA", "SIN EFECTO"} {
		if strings.Contains(t, x) {
			return true
		}
	}
	return false
}
func main() {
	inicio := time.Now()
	labelsFile, err := os.Open("data/etiquetas_procesos.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer labelsFile.Close()
	lr := csv.NewReader(bufio.NewReaderSize(labelsFile, 1<<20))
	lr.ReuseRecord = true
	headers, err := lr.Read()
	if err != nil {
		log.Fatal(err)
	}
	k, cl := indice(headers, "main_ocid"), indice(headers, "riesgo")
	etiquetas := make(map[string]string, 2500000)
	for {
		fila, e := lr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		if fila[cl] == "0" || fila[cl] == "1" {
			etiquetas[strings.TrimSpace(fila[k])] = fila[cl]
		}
	}
	f, err := os.Open("data/main.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	cab, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}
	ocid := indice(cab, "ocid")
	metodo := indice(cab, "tender_procurementMethod")
	categoria := indice(cab, "tender_mainProcurementCategory")
	fecha := indice(cab, "tender_datePublished")
	buyer := indice(cab, "buyer_id")
	detalle := indice(cab, "tender_procurementMethodDetails")
	titulo := indice(cab, "tender_title")
	descripcion := indice(cab, "tender_description")
	outPath := "data/entrenamiento_controlado.csv"
	out, err := os.Create(outPath)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	buf := bufio.NewWriterSize(out, 1<<20)
	w := csv.NewWriter(buf)
	if err := w.Write([]string{"ocid", "metodo", "categoria", "fecha_publicacion", "buyer_id", "metodo_detalle", "titulo", "descripcion", "riesgo"}); err != nil {
		log.Fatal(err)
	}
	vistos := make(map[string]struct{}, len(etiquetas))
	var leidos, escritos, txtDescripciones, descartesTitulo, descartesDescripcion int64
	for {
		fila, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			log.Fatal(e)
		}
		leidos++
		key := strings.TrimSpace(fila[ocid])
		y, ok := etiquetas[key]
		if !ok {
			continue
		}
		if _, seen := vistos[key]; seen {
			continue
		}
		f := strings.TrimSpace(fila[fecha])
		if len(f) < 4 {
			continue
		}
		anio, errAnio := strconv.Atoi(f[:4])
		if errAnio != nil || anio < 2018 || anio > 2025 {
			continue
		}
		vistos[key] = struct{}{}
		t := textoCorto(fila[titulo], 250)
		d := textoCorto(fila[descripcion], 400)
		if sospechoso(t) {
			t = ""
			descartesTitulo++
		}
		if sospechoso(d) {
			d = ""
			descartesDescripcion++
		}
		if d != "" {
			txtDescripciones++
		}
		if err := w.Write([]string{key, fila[metodo], fila[categoria], fila[fecha], fila[buyer], fila[detalle], t, d, y}); err != nil {
			log.Fatal(err)
		}
		escritos++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	if err := buf.Flush(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("=== DATASET CONTROLADO (SIN MONTO/PRESUPUESTO/POSTORES) ===")
	fmt.Printf("Main: %d | Etiquetas: %d | Exportados (2018-2025): %d\n", leidos, len(etiquetas), escritos)
	fmt.Printf("Descripciones no vacias: %d | Titulos descartados por terminos de resultado: %d | Descripciones descartadas: %d\n", txtDescripciones, descartesTitulo, descartesDescripcion)
	fmt.Printf("Archivo: %s | Tiempo: %v\n", outPath, time.Since(inicio))
	fmt.Println("PRECAUCION: Candidatos extraidos de CSV consolidado. No demuestra valores existentes al inicio.")
}
