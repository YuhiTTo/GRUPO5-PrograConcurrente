// Experimento controlado sobre los DOS CSV originales: sin monto ni presupuesto.
// Compara la contribucion de tender_description contra un modelo sin esa descripcion.
// El CSV consolidado NO prueba disponibilidad temporal al momento de convocatoria.
// Uso desde la raiz: go run .\experimentos\entrenar_paralelo_controlado.go -desde 2018 -validacion 2023 -workers 4 -epocas 80 -tasa 0.03 -usar-descripcion=true
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
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	nMetodo        = 64
	nCategoria     = 32
	nComprador     = 4096
	nDetalle       = 128
	nTexto         = 4096
	nDescripcion   = 4096
	nCombinacion   = 512
	nMes           = 12
	maxTokens      = 12
	maxTokensDesc  = 24
	offMes         = 1
	offMetodo      = offMes + nMes
	offCategoria   = offMetodo + nMetodo
	offComprador   = offCategoria + nCategoria
	offDetalle     = offComprador + nComprador
	offTexto       = offDetalle + nDetalle
	offCombinacion = offTexto + nTexto
	offDescripcion = offCombinacion + nCombinacion
	dimension      = offDescripcion + nDescripcion
)

type Ejemplo struct {
	metodo      uint16
	categoria   uint16
	comprador   uint16
	detalle     uint16
	mes         uint8
	combinacion uint16
	tokens      [maxTokens]uint16
	nTokens     uint8
	tokensDesc  [maxTokensDesc]uint16
	nTokensDesc uint8
	clase       uint8
}
type Parcial struct {
	grad [dimension]float64
	peso float64
}
type Metricas struct {
	Total     int     `json:"total"`
	VP        int     `json:"vp"`
	VN        int     `json:"vn"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Exactitud float64 `json:"exactitud"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}
type Registro struct {
	Epoca          int     `json:"epoca"`
	LossValidacion float64 `json:"loss_validacion"`
	AUCValidacion  float64 `json:"auc_roc_validacion"`
}
type Guardado struct {
	Descripcion            string     `json:"descripcion"`
	Desde                  int        `json:"anio_inicio_entrenamiento"`
	Hasta                  int        `json:"anio_fin_entrenamiento"`
	AnioValidacion         int        `json:"anio_validacion"`
	Workers                int        `json:"workers"`
	EpocaMejor             int        `json:"mejor_epoca"`
	PesoPositivo           float64    `json:"peso_positivo"`
	Tasa                   float64    `json:"tasa_adam"`
	Umbral                 float64    `json:"umbral_seleccionado_en_validacion"`
	RegistrosEntrenamiento int        `json:"registros_entrenamiento"`
	RegistrosValidacion    int        `json:"registros_validacion"`
	UsaDescripcion         bool       `json:"usa_descripcion"`
	Historico              []Registro `json:"historial"`
	Validacion             Metricas   `json:"validacion"`
	AUCValidacion          float64    `json:"auc_roc_validacion"`
	APValidacion           float64    `json:"average_precision_validacion"`
	AUCPrueba              *float64   `json:"auc_roc_prueba_exploratoria,omitempty"`
	APPrueba               *float64   `json:"average_precision_prueba_exploratoria,omitempty"`
	Prueba                 *Metricas  `json:"prueba_exploratoria,omitempty"`
	Pesos                  []float64  `json:"pesos"`
}

func idx(c []string, n string) int {
	for i, s := range c {
		if strings.TrimSpace(strings.TrimPrefix(s, "\ufeff")) == n {
			return i
		}
	}
	log.Fatalf("Falta columna: %s", n)
	return -1
}
func normal(s string) string {
	return strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N").Replace(strings.ToUpper(strings.TrimSpace(s)))
}
func hash(s string) uint32 { h := fnv.New32a(); _, _ = h.Write([]byte(s)); return h.Sum32() }
func familia(s string) string {
	v := normal(s)
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
func metodoEstable(general, detalle string) string {
	g := normal(general)
	if g == "OPEN" || g == "DIRECT" || g == "SELECTIVE" || g == "LIMITED" {
		return g
	}
	switch familia(detalle) {
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

var palabrasVacias = map[string]bool{
	"DE": true, "LA": true, "EL": true, "LOS": true, "LAS": true, "DEL": true, "AL": true, "PARA": true,
	"POR": true, "CON": true, "EN": true, "Y": true, "O": true, "UN": true, "UNA": true, "SE": true,
	"QUE": true, "SU": true, "SUS": true, "A": true, "DURANTE": true, "NO": true,
	"CONTRATACION": true, "ADQUISICION": true, "SERVICIO": true, "SERVICIOS": true,
	"BIEN": true, "BIENES": true, "PUBLICA": true, "PUBLICO": true, "EJECUCION": true,
	"ANO": true, "ANIO": true, "MEDIANTE": true,
}

func extraerTokens(titulo string) (out [maxTokens]uint16, n uint8) {
	text := normal(titulo)
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) })
	seen := make(map[uint16]bool, maxTokens)
	for _, w := range words {
		if len([]rune(w)) < 4 || len(w) > 32 || palabrasVacias[w] {
			continue
		}
		id := uint16(hash(w) % nTexto)
		if seen[id] {
			continue
		}
		seen[id] = true
		out[n] = id
		n++
		if n == maxTokens {
			break
		}
	}
	return
}
func extraerTokensDesc(descripcion string) (out [maxTokensDesc]uint16, n uint8) {
	text := normal(descripcion)
	words := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) })
	seen := make(map[uint16]bool, maxTokensDesc)
	for _, w := range words {
		if len([]rune(w)) < 4 || len(w) > 32 || palabrasVacias[w] {
			continue
		}
		id := uint16(hash(w) % nDescripcion)
		if seen[id] {
			continue
		}
		seen[id] = true
		out[n] = id
		n++
		if n == maxTokensDesc {
			break
		}
	}
	return
}
func cargar(ruta string, desde, validacion, testYear int, evaluarPrueba, usarDescripcion bool) (train, val, test []Ejemplo, invalidos int, err error) {
	f, err := os.Open(ruta)
	if err != nil {
		return
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
	r.ReuseRecord = true
	cab, e := r.Read()
	if e != nil {
		err = e
		return
	}
	iID := idx(cab, "ocid")
	iMetodo := idx(cab, "metodo")
	iCat := idx(cab, "categoria")
	iRiesgo := idx(cab, "riesgo")
	iFecha := idx(cab, "fecha_publicacion")
	iBuyer := idx(cab, "buyer_id")
	iDetalle := idx(cab, "metodo_detalle")
	iTitulo := idx(cab, "titulo")
	iDescripcion := idx(cab, "descripcion")
	train = make([]Ejemplo, 0, 600000)
	val = make([]Ejemplo, 0, 90000)
	test = make([]Ejemplo, 0, 90000)
	for {
		fila, readErr := r.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			err = readErr
			return
		}
		fecha := strings.TrimSpace(fila[iFecha])
		if len(fecha) < 7 || fila[iID] == "" || (fila[iRiesgo] != "0" && fila[iRiesgo] != "1") {
			invalidos++
			continue
		}
		year, errYear := strconv.Atoi(fecha[:4])
		mes, errMes := strconv.Atoi(fecha[5:7])
		if errYear != nil || errMes != nil || year < 2004 || year > 2026 || mes < 1 || mes > 12 {
			invalidos++
			continue
		}
		destino := 0
		switch {
		case year >= desde && year < validacion:
			destino = 1
		case year == validacion:
			destino = 2
		case evaluarPrueba && year == testYear:
			destino = 3
		}
		if destino == 0 {
			continue
		}
		familiaDetalle := familia(fila[iDetalle])
		metodo := metodoEstable(fila[iMetodo], fila[iDetalle])
		comprador := strings.TrimSpace(fila[iBuyer])
		categoria := normal(fila[iCat])
		e := Ejemplo{metodo: uint16(hash(metodo) % nMetodo), categoria: uint16(hash(categoria) % nCategoria),
			comprador: uint16(hash(comprador) % nComprador), detalle: uint16(hash(familiaDetalle) % nDetalle),
			combinacion: uint16(hash(categoria+"|"+familiaDetalle) % nCombinacion), mes: uint8(mes - 1)}
		e.tokens, e.nTokens = extraerTokens(fila[iTitulo])
		if usarDescripcion {
			e.tokensDesc, e.nTokensDesc = extraerTokensDesc(fila[iDescripcion])
		}
		if fila[iRiesgo] == "1" {
			e.clase = 1
		}
		switch destino {
		case 1:
			train = append(train, e)
		case 2:
			val = append(val, e)
		case 3:
			test = append(test, e)
		}
	}
	return
}
func sigmoid(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	x := math.Exp(z)
	return x / (1 + x)
}
func puntaje(e *Ejemplo, w []float64) float64 {
	z := w[0] + w[offMes+int(e.mes)]
	z += w[offMetodo+int(e.metodo)] + w[offCategoria+int(e.categoria)]
	z += w[offComprador+int(e.comprador)] + w[offDetalle+int(e.detalle)]
	z += w[offCombinacion+int(e.combinacion)]
	if e.nTokens > 0 {
		escala := 1 / math.Sqrt(float64(e.nTokens))
		for i := uint8(0); i < e.nTokens; i++ {
			z += escala * w[offTexto+int(e.tokens[i])]
		}
	}
	if e.nTokensDesc > 0 {
		escala := 1 / math.Sqrt(float64(e.nTokensDesc))
		for i := uint8(0); i < e.nTokensDesc; i++ {
			z += escala * w[offDescripcion+int(e.tokensDesc[i])]
		}
	}
	return z
}
func acumular(datos []Ejemplo, w []float64, pesoPositivo float64, ch chan<- *Parcial) {
	p := new(Parcial)
	for i := range datos {
		e := &datos[i]
		peso := 1.0
		if e.clase == 1 {
			peso = pesoPositivo
		}
		delta := (sigmoid(puntaje(e, w)) - float64(e.clase)) * peso
		p.grad[0] += delta
		p.grad[offMes+int(e.mes)] += delta
		p.grad[offMetodo+int(e.metodo)] += delta
		p.grad[offCategoria+int(e.categoria)] += delta
		p.grad[offComprador+int(e.comprador)] += delta
		p.grad[offDetalle+int(e.detalle)] += delta
		p.grad[offCombinacion+int(e.combinacion)] += delta
		if e.nTokens > 0 {
			escala := delta / math.Sqrt(float64(e.nTokens))
			for j := uint8(0); j < e.nTokens; j++ {
				p.grad[offTexto+int(e.tokens[j])] += escala
			}
		}
		if e.nTokensDesc > 0 {
			escala := delta / math.Sqrt(float64(e.nTokensDesc))
			for j := uint8(0); j < e.nTokensDesc; j++ {
				p.grad[offDescripcion+int(e.tokensDesc[j])] += escala
			}
		}
		p.peso += peso
	}
	ch <- p
}
func actualizarLote(datos []Ejemplo, w, m, v []float64, pesoPositivo, tasa float64, workers, iter int) {
	if workers > len(datos) {
		workers = len(datos)
	}
	ch := make(chan *Parcial, workers)
	for k := 0; k < workers; k++ {
		start := k * len(datos) / workers
		end := (k + 1) * len(datos) / workers
		go acumular(datos[start:end], w, pesoPositivo, ch)
	}
	var g [dimension]float64
	total := 0.0
	for k := 0; k < workers; k++ {
		p := <-ch
		total += p.peso
		for j := range g {
			g[j] += p.grad[j]
		}
	}
	if total == 0 {
		log.Fatal("Lote sin peso")
	}
	const b1 = 0.9
	const b2 = 0.999
	const epsilon = 1e-8
	b1Corr := 1 - math.Pow(b1, float64(iter))
	b2Corr := 1 - math.Pow(b2, float64(iter))
	for j := range w {
		val := g[j] / total
		m[j] = b1*m[j] + (1-b1)*val
		v[j] = b2*v[j] + (1-b2)*val*val
		step := tasa * (m[j] / b1Corr) / (math.Sqrt(v[j]/b2Corr) + epsilon)
		w[j] -= step
		if j != 0 {
			w[j] *= (1 - tasa*0.0005)
		} // Regularizacion desacoplada (AdamW)
	}
}
func loss(datos []Ejemplo, w []float64) float64 {
	sum := 0.0
	for i := range datos {
		z := puntaje(&datos[i], w)
		sum += math.Max(0, z) + math.Log1p(math.Exp(-math.Abs(z))) - float64(datos[i].clase)*z
	}
	return sum / float64(len(datos))
}

type Par struct {
	score float64
	clase uint8
}

func aucAP(datos []Ejemplo, w []float64) (auc, ap float64) {
	vals := make([]Par, len(datos))
	pos, neg := 0.0, 0.0
	for i := range datos {
		vals[i] = Par{sigmoid(puntaje(&datos[i], w)), datos[i].clase}
		if datos[i].clase == 1 {
			pos++
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return 0, 0
	}
	sort.Slice(vals, func(i, j int) bool { return vals[i].score > vals[j].score })
	// AUC por grupo de scores iguales y Average Precision por umbrales distintos.
	acumPos, acumNeg := 0.0, 0.0
	for i := 0; i < len(vals); {
		j := i + 1
		var gp, gn float64
		if vals[i].clase == 1 {
			gp++
		} else {
			gn++
		}
		for j < len(vals) && vals[j].score == vals[i].score {
			if vals[j].clase == 1 {
				gp++
			} else {
				gn++
			}
			j++
		}
		auc += gp*(neg-acumNeg-gn) + 0.5*gp*gn
		acumPos += gp
		acumNeg += gn
		ap += (gp / pos) * (acumPos / (acumPos + acumNeg))
		i = j
	}
	return auc / (pos * neg), ap
}
func evaluar(datos []Ejemplo, w []float64, umbral float64) Metricas {
	m := Metricas{Total: len(datos)}
	for i := range datos {
		pred := sigmoid(puntaje(&datos[i], w)) >= umbral
		e := datos[i]
		switch {
		case pred && e.clase == 1:
			m.VP++
		case pred && e.clase == 0:
			m.FP++
		case !pred && e.clase == 1:
			m.FN++
		default:
			m.VN++
		}
	}
	if m.Total > 0 {
		m.Exactitud = float64(m.VP+m.VN) / float64(m.Total)
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
	return m
}
func elegirUmbral(datos []Ejemplo, w []float64) (float64, Metricas) {
	// Aproxima el maximo F1 sobre validacion sin usar prueba.
	// Las probabilidades se calculan una sola vez para los 99 umbrales.
	var tp, fp, tn, fn [101]int
	for i := range datos {
		score := sigmoid(puntaje(&datos[i], w))
		y := datos[i].clase
		for j := 1; j <= 99; j++ {
			pred := score >= float64(j)/100.0
			switch {
			case pred && y == 1:
				tp[j]++
			case pred && y == 0:
				fp[j]++
			case !pred && y == 1:
				fn[j]++
			default:
				tn[j]++
			}
		}
	}
	bestU := 0.5
	bestM := Metricas{F1: -1}
	for j := 1; j <= 99; j++ {
		m := Metricas{Total: len(datos), VP: tp[j], FP: fp[j], VN: tn[j], FN: fn[j]}
		if m.VP+m.FP > 0 {
			m.Precision = float64(m.VP) / float64(m.VP+m.FP)
		}
		if m.VP+m.FN > 0 {
			m.Recall = float64(m.VP) / float64(m.VP+m.FN)
		}
		if m.Precision+m.Recall > 0 {
			m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
		}
		m.Exactitud = float64(m.VP+m.VN) / float64(len(datos))
		if m.F1 > bestM.F1+1e-10 {
			bestM = m
			bestU = float64(j) / 100
		}
	}
	return bestU, bestM
}
func main() {
	workers := flag.Int("workers", 4, "Goroutines que calculan gradientes")
	desde := flag.Int("desde", 2019, "Primero anio incluido en entrenamiento")
	validYear := flag.Int("validacion", 2024, "Anio reservado para elegir epoca y umbral")
	testYear := flag.Int("prueba", 2025, "Anio para evaluacion exploratoria, opcional")
	evaluarTest := flag.Bool("evaluar-prueba", false, "Permite evaluar el periodo exploratorio, NUNCA usarlo para sintonizar")
	usarDescripcion := flag.Bool("usar-descripcion", false, "Agregar texto de tender_description (comparacion controlada)")
	epocas := flag.Int("epocas", 80, "Epocas maximas")
	tasa := flag.Float64("tasa", 0.03, "Tasa de aprendizaje Adam")
	pesoPositivo := flag.Float64("peso-positivo", 1.0, "Peso para clase 1; 1.0 significa sin ponderacion")
	lote := flag.Int("lote", 8192, "Tamano del mini-lote de Adam")
	paciencia := flag.Int("paciencia", 12, "Parada temprana en AUC de validacion")
	flag.Parse()
	if *workers < 1 || *epocas < 1 || *tasa <= 0 || *tasa > 1 || *desde < 2004 || *desde >= *validYear || *validYear > 2026 || *lote < *workers || *pesoPositivo <= 0 || *paciencia < 1 {
		log.Fatal("Parametros invalidos")
	}
	inicio := time.Now()
	train, val, test, invalidos, err := cargar("data/entrenamiento_controlado.csv", *desde, *validYear, *testYear, *evaluarTest, *usarDescripcion)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Train (%d-%d): %d | Validacion (%d): %d | Prueba: %d | Invalidos: %d\n", *desde, *validYear-1, len(train), *validYear, len(val), len(test), invalidos)
	fmt.Printf("Carga: %v | Dimensiones: %d | Descripcion activada: %t\n", time.Since(inicio), dimension, *usarDescripcion)
	if len(train) == 0 || len(val) == 0 {
		log.Fatal("Faltan datos de entrenamiento o validacion")
	}
	countPos := 0
	for i := range train {
		if train[i].clase == 1 {
			countPos++
		}
	}
	if countPos == 0 || countPos == len(train) {
		log.Fatal("Se requieren ambas clases")
	}
	fmt.Printf("Train clase 0=%d clase 1=%d | Peso positivo=%.2f\n", len(train)-countPos, countPos, *pesoPositivo)
	w := make([]float64, dimension)
	m := make([]float64, dimension)
	v := make([]float64, dimension)
	mejor := make([]float64, dimension)
	mejorAUC := -1.0
	mejorEpoca := 0
	noMejora := 0
	history := make([]Registro, 0, *epocas)
	gen := rand.New(rand.NewSource(42))
	iter := 0
	startTrain := time.Now()
	for epoca := 1; epoca <= *epocas; epoca++ {
		gen.Shuffle(len(train), func(i, j int) { train[i], train[j] = train[j], train[i] })
		for i := 0; i < len(train); i += *lote {
			fin := i + *lote
			if fin > len(train) {
				fin = len(train)
			}
			iter++
			actualizarLote(train[i:fin], w, m, v, *pesoPositivo, *tasa, *workers, iter)
		}
		auc, _ := aucAP(val, w)
		valLoss := loss(val, w)
		history = append(history, Registro{epoca, valLoss, auc})
		if auc > mejorAUC+0.00005 {
			mejorAUC = auc
			mejorEpoca = epoca
			copy(mejor, w)
			noMejora = 0
		} else {
			noMejora++
		}
		if epoca == 1 || epoca%5 == 0 || noMejora == *paciencia {
			fmt.Printf("Epoca %d/%d | loss val %.5f | AUC val %.5f | mejor AUC %.5f (ep. %d)\n", epoca, *epocas, valLoss, auc, mejorAUC, mejorEpoca)
		}
		if noMejora >= *paciencia {
			fmt.Printf("Parada temprana: %d epocas sin mejora de AUC validacion\n", *paciencia)
			break
		}
	}
	dur := time.Since(startTrain)
	umbral, metVal := elegirUmbral(val, mejor)
	aucVal, apVal := aucAP(val, mejor)
	positivosVal := metVal.VP + metVal.FN
	f1Todos := 2 * float64(positivosVal) / float64(len(val)+positivosVal)
	fmt.Println("\n=== MODELO CONTROLADO SIN MONTO: RESULTADOS VALIDACION ===")
	fmt.Printf("Mejor epoca: %d | Entrenamiento total: %v | AUC: %.4f | Average Precision: %.4f\n", mejorEpoca, dur, aucVal, apVal)
	fmt.Printf("Umbral (validacion): %.2f | F1 val: %.4f | Precision: %.4f | Recall: %.4f\n", umbral, metVal.F1, metVal.Precision, metVal.Recall)
	fmt.Printf("Referencia siempre clase 1 en validacion: F1 %.4f | AUC azar: 0.5000\n", f1Todos)
	if metVal.F1 <= f1Todos {
		fmt.Println("ADVERTENCIA: El modelo no supera la referencia trivial en F1 de validacion.")
	}
	if aucVal < 0.65 {
		fmt.Println("ADVERTENCIA: AUC de validacion debil; revisar etiquetas y caracteristicas.")
	}
	guardado := Guardado{
		Descripcion: "Prototipo controlado sin monto, presupuesto, postores ni estados como predictores. Fuente consolidada: disponibilidad temporal no comprobada.",
		Desde:       *desde, Hasta: *validYear - 1, AnioValidacion: *validYear, Workers: *workers, EpocaMejor: mejorEpoca,
		PesoPositivo: *pesoPositivo, Tasa: *tasa, Umbral: umbral, RegistrosEntrenamiento: len(train), RegistrosValidacion: len(val),
		Historico: history, Validacion: metVal, AUCValidacion: aucVal, APValidacion: apVal, Pesos: mejor, UsaDescripcion: *usarDescripcion,
	}
	if *evaluarTest && len(test) > 0 {
		mTest := evaluar(test, mejor, umbral)
		auc, ap := aucAP(test, mejor)
		fmt.Printf("\nEXPLORATORIO (%d): AUC %.4f AP %.4f F1 %.4f Precision %.4f Recall %.4f | VP %d VN %d FP %d FN %d\n", *testYear, auc, ap, mTest.F1, mTest.Precision, mTest.Recall, mTest.VP, mTest.VN, mTest.FP, mTest.FN)
		guardado.Prueba = &mTest
		guardado.AUCPrueba = &auc
		guardado.APPrueba = &ap
	}
	path := fmt.Sprintf("data/modelo_controlado_desde%d_val%d_desc%t_peso%.2f.json", *desde, *validYear, *usarDescripcion, *pesoPositivo)
	content, err := json.MarshalIndent(guardado, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err = os.WriteFile(path, content, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Modelo guardado:", path)
}
