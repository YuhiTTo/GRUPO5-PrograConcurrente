// Random Forest exploratorio (Go puro, sin dependencias externas).
// Compara un modelo NO lineal con la regresion logistica controlada,
// usando data/entrenamiento_controlado.csv SIN montos, presupuesto o postores.
// CSV consolidado: disponibilidad temporal de textos/metodo NO comprobada.
// Ejecutar desde la raiz del proyecto, cada programa Go por separado:
// go run .\experimentos\entrenar_random_forest_controlado.go -desde 2019 -validacion 2024 -arboles 60 -workers 4 -usar-descripcion=true
package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	nMes         = 12
	nMetodo      = 64
	nCategoria   = 32
	nBuyer       = 4096
	nDetalle     = 128
	nTitulo      = 4096
	nCombo       = 512
	nDesc        = 4096
	offMes       = 0
	offMetodo    = offMes + nMes
	offCategoria = offMetodo + nMetodo
	offBuyer     = offCategoria + nCategoria
	offDetalle   = offBuyer + nBuyer
	offTitulo    = offDetalle + nDetalle
	offCombo     = offTitulo + nTitulo
	offDesc      = offCombo + nCombo
	nFeatures    = offDesc + nDesc
	maxFeatures  = 48
)

type Sample struct {
	Feat [maxFeatures]uint16
	N    uint8
	Y    uint8
}
type Node struct {
	Feature     int     `json:"caracteristica"`
	Left        int     `json:"izq"`
	Right       int     `json:"der"`
	Probability float64 `json:"probabilidad"`
	Size        int     `json:"n"`
}
type Tree struct {
	Nodes []Node `json:"nodos"`
}
type Metrics struct {
	Total     int     `json:"total"`
	VP        int     `json:"vp"`
	VN        int     `json:"vn"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
	Exactitud float64 `json:"exactitud"`
}
type Output struct {
	Description    string   `json:"descripcion"`
	Seed           int64    `json:"semilla"`
	StartYear      int      `json:"desde"`
	ValidYear      int      `json:"validacion"`
	TestYear       int      `json:"prueba,omitempty"`
	TrainSize      int      `json:"n_train"`
	ValSize        int      `json:"n_validacion"`
	TestSize       int      `json:"n_prueba"`
	UseDescription bool     `json:"usa_descripcion"`
	TreesCount     int      `json:"arboles"`
	SampleSize     int      `json:"muestra_por_arbol"`
	MaxDepth       int      `json:"profundidad_maxima"`
	MinLeaf        int      `json:"min_hoja"`
	Mtry           int      `json:"candidatos_por_nodo"`
	Workers        int      `json:"workers"`
	DurationMs     int64    `json:"tiempo_entrenamiento_ms"`
	Threshold      float64  `json:"umbral_en_validacion"`
	AUCRocVal      float64  `json:"auc_validacion"`
	APVal          float64  `json:"ap_validacion"`
	MetricsVal     Metrics  `json:"metricas_validacion"`
	AUCRocTest     *float64 `json:"auc_prueba_exploratoria,omitempty"`
	APTest         *float64 `json:"ap_prueba_exploratoria,omitempty"`
	MetricsTest    *Metrics `json:"metricas_prueba_exploratoria,omitempty"`
	Forest         []Tree   `json:"bosque"`
}

func col(h []string, name string) int {
	for i, s := range h {
		if strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")) == name {
			return i
		}
	}
	log.Fatalf("No existe columna %q", name)
	return 0
}
func norm(s string) string {
	return strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N").Replace(strings.ToUpper(strings.TrimSpace(s)))
}
func hash(s string) uint32 { h := fnv.New32a(); _, _ = h.Write([]byte(s)); return h.Sum32() }
func familia(s string) string {
	v := norm(s)
	switch {
	case strings.Contains(v, "LICITACION"):
		return "LICITACION"
	case strings.Contains(v, "CONCURSO"):
		return "CONCURSO"
	case strings.Contains(v, "ADJUDICACION"):
		return "ADJUDICACION"
	case strings.Contains(v, "SUBASTA"):
		return "SUBASTA"
	case strings.Contains(v, "COMPARACION"):
		return "COMPARACION"
	case strings.Contains(v, "CONTRATACION DIRECTA") || strings.Contains(v, "NO COMPETIT"):
		return "DIRECTA"
	case strings.Contains(v, "REGIMEN"):
		return "REGIMEN"
	case strings.Contains(v, "CONVENIO"):
		return "CONVENIO"
	case strings.Contains(v, "INTERNACIONAL"):
		return "INTERNACIONAL"
	case strings.Contains(v, "MENOR CUANTIA"):
		return "MENOR_CUANTIA"
	case v == "":
		return "SIN_DETALLE"
	default:
		return "OTROS"
	}
}
func metodoEstable(gen, detail string) string {
	g := norm(gen)
	if g == "OPEN" || g == "DIRECT" || g == "SELECTIVE" || g == "LIMITED" {
		return g
	}
	switch familia(detail) {
	case "DIRECTA":
		return "DIRECT"
	case "LICITACION", "CONCURSO", "ADJUDICACION", "SUBASTA", "COMPARACION":
		return "OPEN"
	case "REGIMEN":
		return "REGIMEN"
	case "CONVENIO":
		return "CONVENIO"
	default:
		return "SIN_METODO"
	}
}

var stop = map[string]bool{"DE": true, "LA": true, "EL": true, "LOS": true, "LAS": true, "DEL": true, "AL": true, "PARA": true, "POR": true, "CON": true, "EN": true, "Y": true, "O": true, "UN": true, "UNA": true, "SE": true, "QUE": true, "SU": true, "SUS": true, "A": true, "DURANTE": true, "NO": true, "CONTRATACION": true, "ADQUISICION": true, "SERVICIO": true, "SERVICIOS": true, "BIEN": true, "BIENES": true, "PUBLICA": true, "PUBLICO": true, "EJECUCION": true, "ANO": true, "ANIO": true, "MEDIANTE": true}

func add(s *Sample, feat uint16) {
	if int(s.N) >= maxFeatures {
		return
	}
	s.Feat[s.N] = feat
	s.N++
}
func words(s *Sample, text string, offset, bins, maxWords int) {
	seen := make(map[uint16]bool, maxWords)
	n := 0
	for _, w := range strings.FieldsFunc(norm(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) < 4 || len(w) > 32 || stop[w] {
			continue
		}
		feat := uint16(offset + int(hash(w)%uint32(bins)))
		if seen[feat] {
			continue
		}
		seen[feat] = true
		add(s, feat)
		n++
		if n >= maxWords {
			break
		}
	}
}
func load(path string, startYear, valYear, testYear int, testing, desc bool) (train, val, test []Sample, err error) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	h, e := r.Read()
	if e != nil {
		err = e
		return
	}
	iy, im, ic, ib, id, itt, ides, il, ifec := col(h, "riesgo"), col(h, "metodo"), col(h, "categoria"), col(h, "buyer_id"), col(h, "metodo_detalle"), col(h, "titulo"), col(h, "descripcion"), col(h, "ocid"), col(h, "fecha_publicacion")
	train = make([]Sample, 0, 350000)
	val = make([]Sample, 0, 80000)
	test = make([]Sample, 0, 80000)
	for {
		x, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			err = e
			return
		}
		date := strings.TrimSpace(x[ifec])
		if len(date) < 7 || x[il] == "" || (x[iy] != "0" && x[iy] != "1") {
			continue
		}
		year, ey := strconv.Atoi(date[:4])
		month, em := strconv.Atoi(date[5:7])
		if ey != nil || em != nil || month < 1 || month > 12 {
			continue
		}
		selected := 0
		switch {
		case year >= startYear && year < valYear:
			selected = 1
		case year == valYear:
			selected = 2
		case testing && year == testYear:
			selected = 3
		}
		if selected == 0 {
			continue
		}
		var s Sample
		family := familia(x[id])
		method := metodoEstable(x[im], x[id])
		category := norm(x[ic])
		buyer := strings.TrimSpace(x[ib])
		add(&s, uint16(offMes+month-1))
		add(&s, uint16(offMetodo+int(hash(method)%nMetodo)))
		add(&s, uint16(offCategoria+int(hash(category)%nCategoria)))
		add(&s, uint16(offBuyer+int(hash(buyer)%nBuyer)))
		add(&s, uint16(offDetalle+int(hash(family)%nDetalle)))
		add(&s, uint16(offCombo+int(hash(category+"|"+family)%nCombo)))
		words(&s, x[itt], offTitulo, nTitulo, 12)
		if desc {
			words(&s, x[ides], offDesc, nDesc, 24)
		}
		s.Y = uint8(x[iy][0] - '0')
		sort.Slice(s.Feat[:s.N], func(a, b int) bool { return s.Feat[a] < s.Feat[b] })
		// Remove duplicate feature IDs from same sample.
		n := 0
		for j := 0; j < int(s.N); j++ {
			if n == 0 || s.Feat[j] != s.Feat[n-1] {
				s.Feat[n] = s.Feat[j]
				n++
			}
		}
		s.N = uint8(n)
		switch selected {
		case 1:
			train = append(train, s)
		case 2:
			val = append(val, s)
		case 3:
			test = append(test, s)
		}
	}
	return
}
func has(s *Sample, feature uint16) bool {
	n := int(s.N)
	i := sort.Search(n, func(j int) bool { return s.Feat[j] >= feature })
	return i < n && s.Feat[i] == feature
}
func gini(positive, total int) float64 {
	if total <= 0 {
		return 0
	}
	p := float64(positive) / float64(total)
	return 2 * p * (1 - p)
}

type Trainer struct {
	data                    []Sample
	rng                     *rand.Rand
	maxDepth, minLeaf, mtry int
	counts, pos, stamp      []int32
	epoch                   int32
}

func (tr *Trainer) build(tree *Tree, indices []int32, depth int) int {
	index := len(tree.Nodes)
	tree.Nodes = append(tree.Nodes, Node{Feature: -1, Left: -1, Right: -1, Size: len(indices)})
	positives := 0
	for _, ii := range indices {
		positives += int(tr.data[ii].Y)
	}
	// Laplace smoothing, slightly shrinks small leaves toward 0.5.
	tree.Nodes[index].Probability = (float64(positives) + 1) / (float64(len(indices)) + 2)
	if depth >= tr.maxDepth || len(indices) < 2*tr.minLeaf || positives == 0 || positives == len(indices) {
		return index
	}
	tr.epoch++
	epoch := tr.epoch
	seen := make([]uint16, 0, 1024)
	for _, ii := range indices {
		s := &tr.data[ii]
		for j := 0; j < int(s.N); j++ {
			f := s.Feat[j]
			if tr.stamp[f] != epoch {
				tr.stamp[f] = epoch
				tr.counts[f] = 0
				tr.pos[f] = 0
				seen = append(seen, f)
			}
			tr.counts[f]++
			tr.pos[f] += int32(s.Y)
		}
	}
	eligible := seen[:0]
	for _, f := range seen {
		n := int(tr.counts[f])
		if n >= tr.minLeaf && len(indices)-n >= tr.minLeaf {
			eligible = append(eligible, f)
		}
	}
	if len(eligible) == 0 {
		return index
	}
	// Random subset of eligible split features at each node (Random Forest).
	candCount := tr.mtry
	if candCount > len(eligible) {
		candCount = len(eligible)
	}
	// Shuffle candidate array only when needed; partial Fisher-Yates.
	for j := 0; j < candCount; j++ {
		k := j + tr.rng.Intn(len(eligible)-j)
		eligible[j], eligible[k] = eligible[k], eligible[j]
	}
	initial := gini(positives, len(indices))
	bestGain := 0.0
	bestFeat := -1
	for _, f := range eligible[:candCount] {
		leftN := int(tr.counts[f])
		leftP := int(tr.pos[f])
		rightN := len(indices) - leftN
		rightP := positives - leftP
		gain := initial - (float64(leftN)*gini(leftP, leftN)+float64(rightN)*gini(rightP, rightN))/float64(len(indices))
		if gain > bestGain+1e-10 {
			bestGain = gain
			bestFeat = int(f)
		}
	}
	if bestFeat < 0 || bestGain < 1e-6 {
		return index
	}
	left := make([]int32, 0, len(indices)/2)
	right := make([]int32, 0, len(indices)/2)
	for _, ii := range indices {
		if has(&tr.data[ii], uint16(bestFeat)) {
			left = append(left, ii)
		} else {
			right = append(right, ii)
		}
	}
	if len(left) < tr.minLeaf || len(right) < tr.minLeaf {
		return index
	}
	leftIndex := tr.build(tree, left, depth+1)
	rightIndex := tr.build(tree, right, depth+1)
	tree.Nodes[index].Feature = bestFeat
	tree.Nodes[index].Left = leftIndex
	tree.Nodes[index].Right = rightIndex
	return index
}
func newTree(data []Sample, seed int64, nSample, maxDepth, minLeaf, mtry int) Tree {
	rng := rand.New(rand.NewSource(seed))
	indices := make([]int32, nSample)
	for i := range indices {
		indices[i] = int32(rng.Intn(len(data)))
	}
	trainer := Trainer{data: data, rng: rng, maxDepth: maxDepth, minLeaf: minLeaf, mtry: mtry, counts: make([]int32, nFeatures), pos: make([]int32, nFeatures), stamp: make([]int32, nFeatures)}
	tree := Tree{Nodes: make([]Node, 0, 1<<uint(maxDepth))}
	trainer.build(&tree, indices, 0)
	return tree
}
func treePredict(t *Tree, s *Sample) float64 {
	node := 0
	for {
		n := &t.Nodes[node]
		if n.Feature < 0 {
			return n.Probability
		}
		if has(s, uint16(n.Feature)) {
			node = n.Left
		} else {
			node = n.Right
		}
	}
}
func predictAll(forest []Tree, data []Sample) []float64 {
	out := make([]float64, len(data))
	for i := range data {
		p := 0.0
		for j := range forest {
			p += treePredict(&forest[j], &data[i])
		}
		out[i] = p / float64(len(forest))
	}
	return out
}

type Pair struct {
	Score float64
	Y     uint8
}

func aucAP(samples []Sample, scores []float64) (auc, ap float64) {
	v := make([]Pair, len(samples))
	pos, neg := 0.0, 0.0
	for i := range v {
		v[i] = Pair{scores[i], samples[i].Y}
		if samples[i].Y == 1 {
			pos++
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return 0, 0
	}
	sort.Slice(v, func(i, j int) bool { return v[i].Score > v[j].Score })
	var seenP, seenN float64
	for i := 0; i < len(v); {
		j := i
		gp, gn := 0.0, 0.0
		for j < len(v) && v[j].Score == v[i].Score {
			if v[j].Y == 1 {
				gp++
			} else {
				gn++
			}
			j++
		}
		auc += gp*(neg-seenN-gn) + 0.5*gp*gn
		seenP += gp
		seenN += gn
		ap += (gp / pos) * (seenP / (seenP + seenN))
		i = j
	}
	return auc / (pos * neg), ap
}
func measure(samples []Sample, scores []float64, threshold float64) Metrics {
	m := Metrics{Total: len(samples)}
	for i, s := range samples {
		pred := scores[i] >= threshold
		switch {
		case pred && s.Y == 1:
			m.VP++
		case pred && s.Y == 0:
			m.FP++
		case !pred && s.Y == 1:
			m.FN++
		default:
			m.VN++
		}
	}
	if m.VP+m.FP > 0 {
		m.Precision = float64(m.VP) / float64(m.VP+m.FP)
	}
	if m.VP+m.FN > 0 {
		m.Recall = float64(m.VP) / float64(m.VP+m.FN)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	if m.Total > 0 {
		m.Exactitud = float64(m.VP+m.VN) / float64(m.Total)
	}
	return m
}
func chooseThreshold(samples []Sample, scores []float64) (float64, Metrics) {
	th, best := 0.5, Metrics{F1: -1}
	for j := 1; j <= 99; j++ {
		v := float64(j) / 100
		m := measure(samples, scores, v)
		if m.F1 > best.F1+1e-10 {
			best = m
			th = v
		}
	}
	return th, best
}

func main() {
	startYear := flag.Int("desde", 2019, "Anio inicial entrenamiento")
	valYear := flag.Int("validacion", 2024, "Anio validacion (se selecciona umbral aqui)")
	testYear := flag.Int("prueba", 2025, "Anio evaluacion exploratoria")
	test := flag.Bool("evaluar-prueba", false, "Evaluar anio de prueba una sola vez despues de seleccionar configuracion")
	useDesc := flag.Bool("usar-descripcion", true, "Incluir palabras de tender_description")
	nTrees := flag.Int("arboles", 60, "Cantidad de arboles")
	workers := flag.Int("workers", 4, "Goroutines para entrenar arboles en paralelo")
	sampleSize := flag.Int("muestra", 100000, "Tamanio bootstrap por arbol")
	depth := flag.Int("profundidad", 10, "Profundidad maxima de los arboles")
	leaf := flag.Int("min-hoja", 60, "Minimo registros por hijo")
	mtry := flag.Int("candidatos", 128, "Numero aleatorio de caracteristicas candidatas por nodo")
	seed := flag.Int64("semilla", 42, "Semilla reproducible")
	saveForest := flag.Bool("guardar-bosque", true, "Guardar tambien nodos del modelo (JSON mas pesado)")
	flag.Parse()
	if *workers < 1 || *nTrees < 1 || *sampleSize < 100 || *depth < 1 || *depth > 15 || *leaf < 2 || *mtry < 1 || *valYear <= *startYear || (*test && *testYear <= *valYear) {
		log.Fatal("Parametros invalidos")
	}
	inicio := time.Now()
	train, val, testData, err := load("data/entrenamiento_controlado.csv", *startYear, *valYear, *testYear, *test, *useDesc)
	if err != nil {
		log.Fatal(err)
	}
	if len(train) == 0 || len(val) == 0 {
		log.Fatal("Sin datos para entrenamiento/validacion")
	}
	fmt.Printf("=== RANDOM FOREST SIN MONTO/PRESUPUESTO/POSTORES ===\nTrain %d-%d: %d | Validacion %d: %d | Prueba: %d\n", *startYear, *valYear-1, len(train), *valYear, len(val), len(testData))
	fmt.Printf("Descripcion: %t | Arboles: %d | Bootstrap: %d | Profundidad: %d | Min hoja: %d | Candidatos: %d | Workers: %d | Semilla: %d\n", *useDesc, *nTrees, *sampleSize, *depth, *leaf, *mtry, *workers, *seed)
	fmt.Printf("Carga: %v\n", time.Since(inicio))
	forest := make([]Tree, *nTrees)
	jobs := make(chan int, *nTrees)
	var wg sync.WaitGroup
	start := time.Now()
	actualWorkers := *workers
	if actualWorkers > *nTrees {
		actualWorkers = *nTrees
	}
	for k := 0; k < actualWorkers; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				forest[j] = newTree(train, *seed+int64(j)*1000003, *sampleSize, *depth, *leaf, *mtry)
			}
		}()
	}
	for i := 0; i < *nTrees; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	duration := time.Since(start)
	nodes := 0
	for _, tree := range forest {
		nodes += len(tree.Nodes)
	}
	fmt.Printf("Entrenamiento: %v | Nodos totales: %d\n", duration, nodes)
	vscores := predictAll(forest, val)
	auc, ap := aucAP(val, vscores)
	threshold, metrics := chooseThreshold(val, vscores)
	positives := 0
	for _, s := range val {
		if s.Y == 1 {
			positives++
		}
	}
	f1All := 2 * float64(positives) / float64(len(val)+positives)
	fmt.Printf("VALIDACION (%d): AUC %.4f | AP %.4f | F1 %.4f | Precision %.4f | Recall %.4f | Umbral %.2f\n", *valYear, auc, ap, metrics.F1, metrics.Precision, metrics.Recall, threshold)
	fmt.Printf("Matriz val: VP=%d VN=%d FP=%d FN=%d | Referencia F1 siempre positivo: %.4f | AP referencia: %.4f\n", metrics.VP, metrics.VN, metrics.FP, metrics.FN, f1All, float64(positives)/float64(len(val)))
	output := Output{Description: "Bosque aleatorio sobre caracteristicas del CSV consolidado; momento de disponibilidad de campos no verificado. Etiqueta proxy desfavorable, no fraude probado.", Seed: *seed, StartYear: *startYear, ValidYear: *valYear, TrainSize: len(train), ValSize: len(val), TestSize: len(testData), UseDescription: *useDesc, TreesCount: *nTrees, SampleSize: *sampleSize, MaxDepth: *depth, MinLeaf: *leaf, Mtry: *mtry, Workers: *workers, DurationMs: duration.Milliseconds(), Threshold: threshold, AUCRocVal: auc, APVal: ap, MetricsVal: metrics}
	if *test && len(testData) > 0 {
		tscores := predictAll(forest, testData)
		testAuc, testAP := aucAP(testData, tscores)
		testMetrics := measure(testData, tscores, threshold)
		output.TestYear = *testYear
		output.AUCRocTest = &testAuc
		output.APTest = &testAP
		output.MetricsTest = &testMetrics
		fmt.Printf("PRUEBA EXPLORATORIA (%d): AUC %.4f | AP %.4f | F1 %.4f | Precision %.4f | Recall %.4f\n", *testYear, testAuc, testAP, testMetrics.F1, testMetrics.Precision, testMetrics.Recall)
		fmt.Printf("Matriz prueba: VP=%d VN=%d FP=%d FN=%d\n", testMetrics.VP, testMetrics.VN, testMetrics.FP, testMetrics.FN)
	}
	if *saveForest {
		output.Forest = forest
	}
	name := fmt.Sprintf("data/modelo_random_forest_desde%d_val%d_desc%t.json", *startYear, *valYear, *useDesc)
	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(name, b, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Guardado: %s | Total: %v\n", name, time.Since(inicio))
	fmt.Println("PRECAUCION: evaluaciones retrospectivas; 2025 ya se exploro antes. No afirmar capacidad prospectiva.")
}
