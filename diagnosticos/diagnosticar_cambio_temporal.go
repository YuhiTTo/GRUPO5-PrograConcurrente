// PC3 - Diagnostico descriptivo del cambio entre 2024 y 2025.
// Uso: go run diagnosticar_cambio_temporal.go
// Lee exclusivamente CSV y JSON existentes. NO modifica los datos, el modelo ni Git.
// Importante: este diagnostico NO debe utilizarse para ajustar hiperparametros sobre 2025.
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	nMetodo     = 64
	nCategoria  = 32
	nBuyer      = 256
	nDetalle    = 128
	dimensionV3 = 4 + nMetodo + nCategoria + nBuyer + nDetalle
)

type Conteo struct{ Total, Adversos int }
type Par struct{ Prob, Clase float64 }
type Anio struct {
	Total, Adversos, MontoDisponible, BuyerVacio, BuyerNoVisto int
	Metodos, Categorias, Detalles                              map[string]*Conteo
	Scores                                                     []Par
}
type ModeloV3 struct {
	Pesos      []float64 `json:"pesos"`
	Umbral     float64   `json:"umbral_seleccionado_en_validacion"`
	MejorEpoca int       `json:"mejor_epoca_validacion"`
	Epocas     int       `json:"epocas_solicitadas"`
}

type Indices struct {
	fecha, clase, monto, montoDisp, metodo, categoria, buyer, detalle int
}

func getIndex(headers []string, key string) (int, error) {
	for i, h := range headers {
		if strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")) == key {
			return i, nil
		}
	}
	return 0, fmt.Errorf("falta columna requerida %q", key)
}
func indexar(h []string) (Indices, error) {
	claves := []string{"fecha_publicacion", "riesgo", "monto_pen", "monto_disponible", "metodo", "categoria", "buyer_id", "metodo_detalle"}
	var a [8]int
	for i, c := range claves {
		j, e := getIndex(h, c)
		if e != nil {
			return Indices{}, e
		}
		a[i] = j
	}
	return Indices{a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7]}, nil
}
func recorrer(ruta string, f func(Indices, []string) error) error {
	archivo, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer archivo.Close()
	lector := csv.NewReader(bufio.NewReaderSize(archivo, 1<<20))
	lector.ReuseRecord = true
	head, err := lector.Read()
	if err != nil {
		return err
	}
	ix, err := indexar(head)
	if err != nil {
		return err
	}
	filaNum := 1
	for {
		fila, e := lector.Read()
		if e == io.EOF {
			break
		}
		filaNum++
		if e != nil {
			return fmt.Errorf("CSV fila %d: %w", filaNum, e)
		}
		if e = f(ix, fila); e != nil {
			return fmt.Errorf("fila %d: %w", filaNum, e)
		}
	}
	return nil
}
func year(s string) int {
	s = strings.TrimSpace(s)
	if len(s) < 4 {
		return 0
	}
	v, e := strconv.Atoi(s[:4])
	if e != nil {
		return 0
	}
	return v
}
func hash32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToUpper(strings.TrimSpace(s))))
	return h.Sum32()
}
func sigmoide(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	ez := math.Exp(z)
	return ez / (1 + ez)
}
func probV3(fila []string, i Indices, pesos []float64) float64 {
	monto, disponible := 0.0, 0.0
	if fila[i.montoDisp] == "1" {
		m, e := strconv.ParseFloat(strings.TrimSpace(fila[i.monto]), 64)
		if e == nil && m > 0 && !math.IsNaN(m) && !math.IsInf(m, 0) {
			disponible = 1
			monto = math.Min(math.Log1p(m), 30) / 20
		}
	}
	a := int(hash32(fila[i.metodo]) % nMetodo)
	b := int(hash32(fila[i.categoria]) % nCategoria)
	c := int(hash32(fila[i.buyer]) % nBuyer)
	d := int(hash32(fila[i.detalle]) % nDetalle)
	z := pesos[0] + pesos[1]*monto + pesos[2]*disponible +
		pesos[4+a] + pesos[4+nMetodo+b] + pesos[4+nMetodo+nCategoria+c] + pesos[4+nMetodo+nCategoria+nBuyer+d]
	return sigmoide(z)
}
func sumar(m map[string]*Conteo, k string, adverso bool) {
	k = strings.TrimSpace(k)
	if k == "" {
		k = "(VACIO)"
	}
	x := m[k]
	if x == nil {
		x = &Conteo{}
		m[k] = x
	}
	x.Total++
	if adverso {
		x.Adversos++
	}
}
func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}
func imprimirTabla(titulo string, a, b *Anio, campo func(*Anio) map[string]*Conteo, top int) {
	fmt.Printf("\n=== %s (2024 vs 2025) ===\n", titulo)
	fmt.Printf("%-40s %10s %9s %10s %9s\n", "VALOR", "N 2024", "% CL1", "N 2025", "% CL1")
	keys := make([]string, 0)
	todos := map[string]bool{}
	for k := range campo(a) {
		todos[k] = true
	}
	for k := range campo(b) {
		todos[k] = true
	}
	for k := range todos {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		v1, v2 := campo(b)[keys[i]], campo(b)[keys[j]]
		n1, n2 := 0, 0
		if v1 != nil {
			n1 = v1.Total
		}
		if v2 != nil {
			n2 = v2.Total
		}
		if n1 != n2 {
			return n1 > n2
		}
		return keys[i] < keys[j]
	})
	if top > 0 && len(keys) > top {
		keys = keys[:top]
	}
	for _, k := range keys {
		s, t := Conteo{}, Conteo{}
		if z := campo(a)[k]; z != nil {
			s = *z
		}
		if z := campo(b)[k]; z != nil {
			t = *z
		}
		nombre := []rune(k)
		if len(nombre) > 40 {
			nombre = append(nombre[:37], '.', '.', '.')
		}
		fmt.Printf("%-40s %10d %8.2f%% %10d %8.2f%%\n", string(nombre), s.Total, pct(s.Adversos, s.Total), t.Total, pct(t.Adversos, t.Total))
	}
	fmt.Printf("Valores distintos (union): %d\n", len(todos))
}
func auc(pares []Par) float64 {
	v := append([]Par(nil), pares...)
	var nPos, nNeg float64
	for _, p := range v {
		if p.Clase == 1 {
			nPos++
		} else {
			nNeg++
		}
	}
	if nPos == 0 || nNeg == 0 {
		return math.NaN()
	}
	sort.Slice(v, func(i, j int) bool { return v[i].Prob < v[j].Prob })
	var negAntes, concordancia float64
	for i := 0; i < len(v); {
		j := i + 1
		p, n := 0.0, 0.0
		if v[i].Clase == 1 {
			p++
		} else {
			n++
		}
		for j < len(v) && v[j].Prob == v[i].Prob {
			if v[j].Clase == 1 {
				p++
			} else {
				n++
			}
			j++
		}
		concordancia += p*negAntes + 0.5*p*n
		negAntes += n
		i = j
	}
	return concordancia / (nPos * nNeg)
}
func imprimirScore(ano int, a *Anio, threshold float64) {
	var vp, vn, fp, fn int
	var sumPos, sumNeg float64
	for _, p := range a.Scores {
		if p.Clase == 1 {
			sumPos += p.Prob
		} else {
			sumNeg += p.Prob
		}
		pred := p.Prob >= threshold
		switch {
		case pred && p.Clase == 1:
			vp++
		case pred && p.Clase == 0:
			fp++
		case !pred && p.Clase == 1:
			fn++
		default:
			vn++
		}
	}
	precision, recall := 0.0, 0.0
	if vp+fp > 0 {
		precision = float64(vp) / float64(vp+fp)
	}
	if vp+fn > 0 {
		recall = float64(vp) / float64(vp+fn)
	}
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	fmt.Printf("%d | umbral %.2f | VP=%d VN=%d FP=%d FN=%d | Precision %.4f Recall %.4f F1 %.4f | pred. positivas %.2f%%\n",
		ano, threshold, vp, vn, fp, fn, precision, recall, f1, pct(vp+fp, len(a.Scores)))
	if threshold == 0.50 {
		pos := a.Adversos
		neg := a.Total - pos
		fmt.Printf("  AUC-ROC %.4f | Score promedio clase 0: %.4f | clase 1: %.4f\n", auc(a.Scores), sumNeg/float64(neg), sumPos/float64(pos))
	}
}
func main() {
	csvRuta := flag.String("csv", "data/entrenamiento_v3.csv", "Archivo de entrenamiento V3")
	modeloRuta := flag.String("modelo", "data/modelo_logistico_v3.json", "Modelo V3 previamente entrenado (idealmente 150 epocas)")
	flag.Parse()

	mod := ModeloV3{}
	predicciones := false
	if contenido, e := os.ReadFile(*modeloRuta); e == nil {
		if e = json.Unmarshal(contenido, &mod); e != nil {
			fmt.Printf("ADVERTENCIA: JSON de modelo invalido (%v); se omiten scores.\n", e)
		} else if len(mod.Pesos) != dimensionV3 {
			fmt.Printf("ADVERTENCIA: pesos esperados %d, encontrados %d; se omiten scores.\n", dimensionV3, len(mod.Pesos))
		} else {
			predicciones = true
		}
	} else {
		fmt.Printf("ADVERTENCIA: no se pudo abrir modelo %s (%v); se muestran solo distribuciones.\n", *modeloRuta, e)
	}

	fmt.Println("=== DIAGNOSTICO TEMPORAL V3 - LECTURA SIN MODIFICACIONES ===")
	if predicciones {
		fmt.Printf("Modelo: %s | Epocas: %d | Mejor epoca: %d | Umbral almacenado: %.2f\n", *modeloRuta, mod.Epocas, mod.MejorEpoca, mod.Umbral)
	}
	fmt.Println("Paso 1/2: identificar compradores observados hasta 2023...")
	compradoresHistoricos := make(map[string]bool)
	if e := recorrer(*csvRuta, func(ix Indices, f []string) error {
		if year(f[ix.fecha]) <= 2023 && year(f[ix.fecha]) >= 2004 && (f[ix.clase] == "0" || f[ix.clase] == "1") {
			k := strings.TrimSpace(f[ix.buyer])
			if k != "" {
				compradoresHistoricos[k] = true
			}
		}
		return nil
	}); e != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", e)
		os.Exit(1)
	}
	fmt.Printf("Compradores distintos hasta 2023: %d\n", len(compradoresHistoricos))

	years := map[int]*Anio{}
	for _, y := range []int{2023, 2024, 2025} {
		years[y] = &Anio{Metodos: make(map[string]*Conteo), Categorias: make(map[string]*Conteo), Detalles: make(map[string]*Conteo)}
	}
	fmt.Println("Paso 2/2: comparar 2023, 2024 y 2025...")
	invalidos := 0
	err := recorrer(*csvRuta, func(ix Indices, f []string) error {
		y := year(f[ix.fecha])
		a := years[y]
		if a == nil {
			return nil
		}
		if f[ix.clase] != "0" && f[ix.clase] != "1" {
			invalidos++
			return nil
		}
		pos := f[ix.clase] == "1"
		a.Total++
		if pos {
			a.Adversos++
		}
		if f[ix.montoDisp] == "1" {
			a.MontoDisponible++
		}
		buyer := strings.TrimSpace(f[ix.buyer])
		if buyer == "" {
			a.BuyerVacio++
		} else if !compradoresHistoricos[buyer] {
			a.BuyerNoVisto++
		}
		sumar(a.Metodos, f[ix.metodo], pos)
		sumar(a.Categorias, f[ix.categoria], pos)
		sumar(a.Detalles, f[ix.detalle], pos)
		if predicciones && (y == 2024 || y == 2025) {
			c := 0.0
			if pos {
				c = 1
			}
			a.Scores = append(a.Scores, Par{Prob: probV3(f, ix, mod.Pesos), Clase: c})
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	fmt.Printf("\n%-6s %10s %10s %10s %12s %12s %14s\n", "ANIO", "TOTAL", "CLASE 1", "% CL1", "% MONTO OK", "% BUYER VAC", "% BUYER NUEVO")
	for _, y := range []int{2023, 2024, 2025} {
		a := years[y]
		fmt.Printf("%-6d %10d %10d %9.2f%% %11.2f%% %11.2f%% %13.2f%%\n", y, a.Total, a.Adversos, pct(a.Adversos, a.Total), pct(a.MontoDisponible, a.Total), pct(a.BuyerVacio, a.Total), pct(a.BuyerNoVisto, a.Total))
	}
	if invalidos > 0 {
		fmt.Printf("Filas invalidas 2023-2025: %d\n", invalidos)
	}
	imprimirTabla("METODO GENERAL", years[2024], years[2025], func(a *Anio) map[string]*Conteo { return a.Metodos }, 10)
	imprimirTabla("CATEGORIA", years[2024], years[2025], func(a *Anio) map[string]*Conteo { return a.Categorias }, 10)
	imprimirTabla("METODO DETALLADO - TOP 12 por volumen 2025", years[2024], years[2025], func(a *Anio) map[string]*Conteo { return a.Detalles }, 12)
	if predicciones {
		fmt.Println("\n=== DIAGNOSTICO DE PREDICCIONES DE LA V3 ===")
		for _, u := range []float64{0.50, 0.55} {
			imprimirScore(2024, years[2024], u)
			imprimirScore(2025, years[2025], u)
		}
		fmt.Println("NOTA: Las metricas de 2025 son SOLO diagnosticas; no ajuste la V4 mirando 2025.")
	}
	fmt.Println("FIN. No se modificaron datos ni pesos del modelo.")
}
