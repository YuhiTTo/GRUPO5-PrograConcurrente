// PC3 - Regresion Logistica: variables adicionales y evaluacion temporal.
// Uso: go run entrenar_paralelo_v3.go -workers 4 -epocas 30 -tasa 0.8
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
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	cubetasMetodo    = 64
	cubetasCategoria = 32
	cubetasBuyer     = 256
	cubetasDetalle   = 128
	dimension        = 4 + cubetasMetodo + cubetasCategoria + cubetasBuyer + cubetasDetalle
)

type Ejemplo struct {
	monto      float64
	disponible float64
	metodo     int
	categoria  int
	buyer      int
	detalle    int
	anio       float64
	clase      float64
}

type Gradiente struct {
	valores          [dimension]float64
	pesoTotal        float64
	perdidaPonderada float64
}

type Metricas struct {
	Total     int     `json:"total"`
	VP        int     `json:"verdaderos_positivos"`
	VN        int     `json:"verdaderos_negativos"`
	FP        int     `json:"falsos_positivos"`
	FN        int     `json:"falsos_negativos"`
	Exactitud float64 `json:"exactitud"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

type HistorialEpoca struct {
	Epoca                  int     `json:"epoca"`
	LossEntrenamientoAntes float64 `json:"loss_entrenamiento_antes_actualizacion"`
	LossValidacionDespues  float64 `json:"loss_validacion_despues_actualizacion"`
}

type Modelo struct {
	Algoritmo              string             `json:"algoritmo"`
	Descripcion            string             `json:"descripcion"`
	Workers                int                `json:"workers"`
	EpocasSolicitadas      int                `json:"epocas_solicitadas"`
	MejorEpoca             int                `json:"mejor_epoca_validacion"`
	Tasa                   float64            `json:"tasa_aprendizaje"`
	PesoPositivo           float64            `json:"peso_clase_1"`
	Umbral                 float64            `json:"umbral_seleccionado_en_validacion"`
	Pesos                  [dimension]float64 `json:"pesos"`
	RegistrosEntrenamiento int                `json:"registros_entrenamiento"`
	RegistrosValidacion    int                `json:"registros_validacion"`
	RegistrosPrueba        int                `json:"registros_prueba"`
	Historial              []HistorialEpoca   `json:"historial_loss"`
	Validacion             Metricas           `json:"evaluacion_validacion"`
	Prueba                 Metricas           `json:"evaluacion_prueba"`
	AUCValidacion          float64            `json:"auc_roc_validacion"`
	AUCPrueba              float64            `json:"auc_roc_prueba"`
}

func indice(cabeceras []string, nombre string) int {
	for i, c := range cabeceras {
		if strings.TrimSpace(c) == nombre {
			return i
		}
	}
	log.Fatalf("No existe la columna requerida: %s", nombre)
	return -1
}

func hash32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToUpper(strings.TrimSpace(s))))
	return h.Sum32()
}

func cargar(ruta string) (train, validacion, test []Ejemplo, invalidos int, err error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 1024*1024))
	r.ReuseRecord = true
	cab, err := r.Read()
	if err != nil {
		return nil, nil, nil, 0, err
	}

	iID := indice(cab, "ocid")
	iMonto := indice(cab, "monto_pen")
	iDisponible := indice(cab, "monto_disponible")
	iMetodo := indice(cab, "metodo")
	iCategoria := indice(cab, "categoria")
	iRiesgo := indice(cab, "riesgo")
	iFecha := indice(cab, "fecha_publicacion")
	iBuyer := indice(cab, "buyer_id")
	iDetalle := indice(cab, "metodo_detalle")

	train = make([]Ejemplo, 0, 2300000)
	validacion = make([]Ejemplo, 0, 100000)
	test = make([]Ejemplo, 0, 100000)

	for {
		fila, eLectura := r.Read()
		if eLectura == io.EOF {
			break
		}
		if eLectura != nil {
			return nil, nil, nil, 0, eLectura
		}

		if (fila[iRiesgo] != "0" && fila[iRiesgo] != "1") || strings.TrimSpace(fila[iID]) == "" {
			invalidos++
			continue
		}

		// La fecha de publicacion se usa SOLO para el corte temporal,
		// no como predictor, para limitar atajos temporales.
		fecha := strings.TrimSpace(fila[iFecha])
		if len(fecha) < 4 {
			invalidos++
			continue
		}
		anio, errAnio := strconv.Atoi(fecha[:4])
		if errAnio != nil || anio < 2004 || anio > 2026 {
			invalidos++
			continue
		}
		if anio == 2026 {
			continue
		} // Procesos recientes pueden estar censurados.
		e := Ejemplo{
			metodo:    int(hash32(fila[iMetodo]) % cubetasMetodo),
			categoria: int(hash32(fila[iCategoria]) % cubetasCategoria),
			buyer:     int(hash32(fila[iBuyer]) % cubetasBuyer),
			detalle:   int(hash32(fila[iDetalle]) % cubetasDetalle),
		}
		if fila[iRiesgo] == "1" {
			e.clase = 1
		}
		if fila[iDisponible] == "1" {
			monto, parseErr := strconv.ParseFloat(strings.TrimSpace(fila[iMonto]), 64)
			if parseErr == nil && monto > 0 && !math.IsNaN(monto) && !math.IsInf(monto, 0) {
				e.disponible = 1
				e.monto = math.Min(math.Log1p(monto), 30) / 20
			}
		}
		switch {
		case anio <= 2023:
			train = append(train, e)
		case anio == 2024:
			validacion = append(validacion, e)
		case anio == 2025:
			test = append(test, e)
		}
	}
	return train, validacion, test, invalidos, nil
}

func sigmoide(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	ez := math.Exp(z)
	return ez / (1 + ez)
}

func puntaje(e Ejemplo, pesos *[dimension]float64) float64 {
	return pesos[0] + pesos[1]*e.monto + pesos[2]*e.disponible +
		pesos[4+e.metodo] + pesos[4+cubetasMetodo+e.categoria] +
		pesos[4+cubetasMetodo+cubetasCategoria+e.buyer] +
		pesos[4+cubetasMetodo+cubetasCategoria+cubetasBuyer+e.detalle]
}

func probabilidad(e Ejemplo, pesos *[dimension]float64) float64 {
	return sigmoide(puntaje(e, pesos))
}

// Log-loss estable: softplus(z) - y*z, ponderado segun la clase.
func entropiaCruzada(z, y float64) float64 {
	return math.Max(0, z) + math.Log1p(math.Exp(-math.Abs(z))) - y*z
}

func gradienteParcial(datos []Ejemplo, pesos *[dimension]float64, pesoPositivo float64, canal chan<- Gradiente) {
	var g Gradiente
	for _, e := range datos {
		peso := 1.0
		if e.clase == 1 {
			peso = pesoPositivo
		}
		z := puntaje(e, pesos)
		delta := (sigmoide(z) - e.clase) * peso
		g.valores[0] += delta
		g.valores[1] += delta * e.monto
		g.valores[2] += delta * e.disponible
		g.valores[4+e.metodo] += delta
		g.valores[4+cubetasMetodo+e.categoria] += delta
		g.valores[4+cubetasMetodo+cubetasCategoria+e.buyer] += delta
		g.valores[4+cubetasMetodo+cubetasCategoria+cubetasBuyer+e.detalle] += delta
		g.pesoTotal += peso
		g.perdidaPonderada += peso * entropiaCruzada(z, e.clase)
	}
	canal <- g
}

func perdida(datos []Ejemplo, pesos *[dimension]float64, pesoPositivo float64) float64 {
	var suma, pesoTotal float64
	for _, e := range datos {
		peso := 1.0
		if e.clase == 1 {
			peso = pesoPositivo
		}
		suma += peso * entropiaCruzada(puntaje(e, pesos), e.clase)
		pesoTotal += peso
	}
	if pesoTotal == 0 {
		return math.NaN()
	}
	return suma / pesoTotal
}

func entrenar(train, validacion []Ejemplo, workers, epocas int, tasa float64) (
	mejorPesos [dimension]float64, pesoPositivo float64, mejorEpoca int,
	historial []HistorialEpoca, tiempoGradientes time.Duration,
) {
	positivos := 0
	for _, e := range train {
		if e.clase == 1 {
			positivos++
		}
	}
	negativos := len(train) - positivos
	if positivos == 0 || negativos == 0 {
		log.Fatal("El entrenamiento necesita ambas clases")
	}
	pesoPositivo = float64(negativos) / float64(positivos)
	fmt.Printf("Entrenamiento: clase 0=%d, clase 1=%d, peso clase 1=%.3f\n", negativos, positivos, pesoPositivo)

	var pesos [dimension]float64
	mejorLoss := math.Inf(1)
	historial = make([]HistorialEpoca, 0, epocas)

	for epoca := 1; epoca <= epocas; epoca++ {
		inicioGradientes := time.Now()
		canal := make(chan Gradiente, workers)
		for worker := 0; worker < workers; worker++ {
			ini := worker * len(train) / workers
			fin := (worker + 1) * len(train) / workers
			go gradienteParcial(train[ini:fin], &pesos, pesoPositivo, canal)
		}

		var gTotal [dimension]float64
		var sumaPesos, sumaPerdidas float64
		for worker := 0; worker < workers; worker++ {
			parcial := <-canal
			sumaPesos += parcial.pesoTotal
			sumaPerdidas += parcial.perdidaPonderada
			for j := range gTotal {
				gTotal[j] += parcial.valores[j]
			}
		}
		if sumaPesos <= 0 {
			log.Fatal("Peso total invalido")
		}
		for j := range pesos {
			reg := 0.0001 * pesos[j]
			if j == 0 {
				reg = 0
			}
			pesos[j] -= tasa * (gTotal[j]/sumaPesos + reg)
		}
		tiempoGradientes += time.Since(inicioGradientes)

		// Validacion se calcula despues de actualizar los pesos.
		lossVal := perdida(validacion, &pesos, pesoPositivo)
		if lossVal < mejorLoss {
			mejorLoss = lossVal
			mejorPesos = pesos
			mejorEpoca = epoca
		}
		h := HistorialEpoca{
			Epoca:                  epoca,
			LossEntrenamientoAntes: sumaPerdidas / sumaPesos,
			LossValidacionDespues:  lossVal,
		}
		historial = append(historial, h)
		if epoca == 1 || epoca%5 == 0 || epoca == epocas {
			fmt.Printf("Epoca %d/%d | loss train (antes): %.5f | loss val (despues): %.5f\n",
				epoca, epocas, h.LossEntrenamientoAntes, h.LossValidacionDespues)
		}
	}
	return mejorPesos, pesoPositivo, mejorEpoca, historial, tiempoGradientes
}

func evaluar(datos []Ejemplo, pesos *[dimension]float64, umbral float64) Metricas {
	m := Metricas{Total: len(datos)}
	for _, e := range datos {
		pred := probabilidad(e, pesos) >= umbral
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

func elegirUmbral(validacion []Ejemplo, pesos *[dimension]float64) (float64, Metricas) {
	mejorUmbral := 0.5
	mejorMetrica := evaluar(validacion, pesos, mejorUmbral)
	// Incluye 0.0: es la referencia 'predecir siempre desfavorable'.
	for i := 0; i <= 20; i++ {
		u := float64(i) / 20.0
		m := evaluar(validacion, pesos, u)
		if m.F1 > mejorMetrica.F1+1e-12 {
			mejorUmbral = u
			mejorMetrica = m
		}
	}
	return mejorUmbral, mejorMetrica
}

type scoreClase struct {
	score float64
	clase float64
}

// AUC-ROC por comparacion de pares con manejo de empates.
func calcularAUC(datos []Ejemplo, pesos *[dimension]float64) float64 {
	pares := make([]scoreClase, len(datos))
	var positivos, negativos float64
	for i, e := range datos {
		pares[i] = scoreClase{score: probabilidad(e, pesos), clase: e.clase}
		if e.clase == 1 {
			positivos++
		} else {
			negativos++
		}
	}
	if positivos == 0 || negativos == 0 {
		return 0
	}
	sort.Slice(pares, func(i, j int) bool { return pares[i].score < pares[j].score })
	var negativosPrevios, concordancia float64
	for i := 0; i < len(pares); {
		j := i + 1
		var grupoPos, grupoNeg float64
		if pares[i].clase == 1 {
			grupoPos++
		} else {
			grupoNeg++
		}
		for j < len(pares) && pares[j].score == pares[i].score {
			if pares[j].clase == 1 {
				grupoPos++
			} else {
				grupoNeg++
			}
			j++
		}
		concordancia += grupoPos*negativosPrevios + 0.5*grupoPos*grupoNeg
		negativosPrevios += grupoNeg
		i = j
	}
	return concordancia / (positivos * negativos)
}

func main() {
	workers := flag.Int("workers", 4, "Numero de goroutines para gradientes")
	epocas := flag.Int("epocas", 30, "Numero de epocas maximas")
	tasa := flag.Float64("tasa", 0.8, "Tasa de aprendizaje")
	flag.Parse()
	if *workers < 1 || *epocas < 1 || *tasa <= 0 || math.IsNaN(*tasa) || math.IsInf(*tasa, 0) {
		log.Fatal("Parametros invalidos")
	}

	inicioCarga := time.Now()
	train, validacion, test, invalidos, err := cargar("data/entrenamiento_v3.csv")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Train: %d | Validacion: %d | Prueba: %d | Invalidos: %d\n", len(train), len(validacion), len(test), invalidos)
	fmt.Println("Tiempo de carga:", time.Since(inicioCarga))
	if len(train) == 0 || len(validacion) == 0 || len(test) == 0 {
		log.Fatal("Particiones insuficientes")
	}

	tiempoCompleto := time.Now()
	pesos, pesoPositivo, mejorEpoca, historial, tiempoGradientes := entrenar(train, validacion, *workers, *epocas, *tasa)
	duracionEntrenamiento := time.Since(tiempoCompleto)
	umbral, metricaVal := elegirUmbral(validacion, &pesos)
	aucVal := calcularAUC(validacion, &pesos)
	metricasTest := evaluar(test, &pesos, umbral)
	aucTest := calcularAUC(test, &pesos)
	baseCero := evaluar(test, &pesos, 1.1) // Nunca predice clase 1.
	baseUno := evaluar(test, &pesos, 0.0)  // Siempre predice clase 1.

	fmt.Println("\n=== REGRESION LOGISTICA PARALELA V3 (CORTE TEMPORAL) ===")
	fmt.Printf("Workers: %d | Epocas solicitadas: %d | Mejor epoca (validacion): %d\n", *workers, *epocas, mejorEpoca)
	fmt.Printf("Tiempo gradientes+actualizaciones: %s | Tiempo total entrenamiento+validacion: %s\n", tiempoGradientes, duracionEntrenamiento)
	fmt.Printf("Umbral seleccionado SOLO en validacion: %.2f | F1 val: %.4f | AUC-ROC val: %.4f\n", umbral, metricaVal.F1, aucVal)
	fmt.Printf("Prueba: exactitud %.4f | precision %.4f | recall %.4f | F1 %.4f | AUC-ROC %.4f\n",
		metricasTest.Exactitud, metricasTest.Precision, metricasTest.Recall, metricasTest.F1, aucTest)
	fmt.Printf("Matriz prueba: VP=%d, VN=%d, FP=%d, FN=%d\n", metricasTest.VP, metricasTest.VN, metricasTest.FP, metricasTest.FN)
	fmt.Printf("Referencia siempre clase 0: exactitud %.4f, F1 %.4f\n", baseCero.Exactitud, baseCero.F1)
	fmt.Printf("Referencia siempre clase 1: exactitud %.4f, F1 %.4f\n", baseUno.Exactitud, baseUno.F1)
	if umbral == 0 {
		fmt.Println("ADVERTENCIA: El mejor umbral es 0.00; el modelo no mejora el F1 de predecir siempre clase 1 en validacion.")
	}

	modelo := Modelo{
		Algoritmo:   "Regresion logistica con gradientes paralelos en Go",
		Descripcion: "Prototipo exploratorio. Entrena hasta 2023, valida 2024, evalua 2025. No apto para alertas reales.",
		Workers:     *workers, EpocasSolicitadas: *epocas, MejorEpoca: mejorEpoca,
		Tasa: *tasa, PesoPositivo: pesoPositivo, Umbral: umbral,
		Pesos:                  pesos,
		RegistrosEntrenamiento: len(train), RegistrosValidacion: len(validacion), RegistrosPrueba: len(test),
		Historial: historial, Validacion: metricaVal, Prueba: metricasTest,
		AUCValidacion: aucVal, AUCPrueba: aucTest,
	}
	contenido, err := json.MarshalIndent(modelo, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	rutaSalida := "data/modelo_logistico_v3.json"
	if err := os.WriteFile(rutaSalida, contenido, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Modelo guardado:", rutaSalida)
}
