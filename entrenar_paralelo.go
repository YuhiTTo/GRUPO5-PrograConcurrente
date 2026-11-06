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
	"strconv"
	"strings"
	"time"
)

const (
	cubetasMetodo    = 64
	cubetasCategoria = 32
	dimension        = 3 + cubetasMetodo + cubetasCategoria
)

// Cada ejemplo guarda cinco valores, en lugar de una matriz densa.
type Ejemplo struct {
	monto      float64
	disponible float64
	metodo     int
	categoria  int
	clase      float64
}

type Gradiente struct {
	valores   [dimension]float64
	pesoTotal float64
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

func enPrueba(id string) bool {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64()%5 == 0 // Separacion reproducible 80/20 por identificador
}

func cargar(ruta string) ([]Ejemplo, []Ejemplo, int, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, nil, 0, err
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 1024*1024))
	r.ReuseRecord = true
	cab, err := r.Read()
	if err != nil {
		return nil, nil, 0, err
	}

	iID := indice(cab, "ocid")
	iMonto := indice(cab, "monto_pen")
	iDisponible := indice(cab, "monto_disponible")
	iMetodo := indice(cab, "metodo")
	iCategoria := indice(cab, "categoria")
	iRiesgo := indice(cab, "riesgo")

	entrenamiento := make([]Ejemplo, 0, 2000000)
	prueba := make([]Ejemplo, 0, 500000)
	invalidos := 0

	for {
		fila, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, 0, err
		}
		if fila[iRiesgo] != "0" && fila[iRiesgo] != "1" {
			invalidos++
			continue
		}
		e := Ejemplo{
			metodo:    int(hash32(fila[iMetodo]) % cubetasMetodo),
			categoria: int(hash32(fila[iCategoria]) % cubetasCategoria),
		}
		if fila[iRiesgo] == "1" {
			e.clase = 1
		}
		if fila[iDisponible] == "1" {
			valor, err := strconv.ParseFloat(strings.TrimSpace(fila[iMonto]), 64)
			if err == nil && valor > 0 && !math.IsNaN(valor) && !math.IsInf(valor, 0) {
				e.disponible = 1
				e.monto = math.Min(math.Log1p(valor), 30) / 20
			}
		}
		if enPrueba(fila[iID]) {
			prueba = append(prueba, e)
		} else {
			entrenamiento = append(entrenamiento, e)
		}
	}
	return entrenamiento, prueba, invalidos, nil
}

func sigmoide(z float64) float64 {
	if z >= 0 {
		return 1 / (1 + math.Exp(-z))
	}
	e := math.Exp(z)
	return e / (1 + e)
}

func probabilidad(e Ejemplo, pesos *[dimension]float64) float64 {
	z := pesos[0] + pesos[1]*e.monto + pesos[2]*e.disponible +
		pesos[3+e.metodo] + pesos[3+cubetasMetodo+e.categoria]
	return sigmoide(z)
}

func calcularGradiente(datos []Ejemplo, pesos *[dimension]float64, pesoPositivo float64, canal chan<- Gradiente) {
	var g Gradiente
	for _, e := range datos {
		peso := 1.0
		if e.clase == 1 {
			peso = pesoPositivo
		}
		errorPred := (probabilidad(e, pesos) - e.clase) * peso
		g.valores[0] += errorPred
		g.valores[1] += errorPred * e.monto
		g.valores[2] += errorPred * e.disponible
		g.valores[3+e.metodo] += errorPred
		g.valores[3+cubetasMetodo+e.categoria] += errorPred
		g.pesoTotal += peso
	}
	canal <- g
}

func entrenar(datos []Ejemplo, workers, epocas int, tasa float64) ([dimension]float64, float64, int, int) {
	var pesos [dimension]float64
	positivos := 0
	for _, e := range datos {
		if e.clase == 1 {
			positivos++
		}
	}
	negativos := len(datos) - positivos
	if positivos == 0 || negativos == 0 {
		log.Fatal("El conjunto de entrenamiento debe contener ambas clases")
	}
	pesoPositivo := float64(negativos) / float64(positivos)
	fmt.Printf("Entrenamiento: clase 0=%d, clase 1=%d, peso clase 1=%.3f\n", negativos, positivos, pesoPositivo)

	for epoca := 1; epoca <= epocas; epoca++ {
		canal := make(chan Gradiente, workers)
		for worker := 0; worker < workers; worker++ {
			inicio := worker * len(datos) / workers
			fin := (worker + 1) * len(datos) / workers
			go calcularGradiente(datos[inicio:fin], &pesos, pesoPositivo, canal)
		}

		var total [dimension]float64
		var sumaPesos float64
		for worker := 0; worker < workers; worker++ {
			parcial := <-canal
			sumaPesos += parcial.pesoTotal
			for j := range total {
				total[j] += parcial.valores[j]
			}
		}
		for j := range pesos {
			// Penalizacion L2 pequeña; no penalizar el intercepto.
			regularizacion := 0.0001 * pesos[j]
			if j == 0 {
				regularizacion = 0
			}
			pesos[j] -= tasa * (total[j]/sumaPesos + regularizacion)
		}
		if epoca == 1 || epoca%5 == 0 || epoca == epocas {
			fmt.Printf("Epoca %d/%d completada\n", epoca, epocas)
		}
	}
	return pesos, pesoPositivo, negativos, positivos
}

func evaluar(datos []Ejemplo, pesos *[dimension]float64) Metricas {
	m := Metricas{Total: len(datos)}
	for _, e := range datos {
		pred := probabilidad(e, pesos) >= 0.5
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

func main() {
	workers := flag.Int("workers", 4, "Numero de goroutines para el entrenamiento")
	epocas := flag.Int("epocas", 30, "Numero de epocas")
	tasa := flag.Float64("tasa", 0.8, "Tasa de aprendizaje")
	flag.Parse()
	if *workers < 1 || *epocas < 1 || *tasa <= 0 {
		log.Fatal("Parametros no validos")
	}

	inicioCarga := time.Now()
	entrenamiento, prueba, invalidos, err := cargar("data/entrenamiento_base.csv")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Entrenamiento: %d registros | Prueba: %d registros | Invalidos: %d\n", len(entrenamiento), len(prueba), invalidos)
	fmt.Println("Tiempo de carga:", time.Since(inicioCarga))
	if len(entrenamiento) == 0 || len(prueba) == 0 {
		log.Fatal("No hay suficientes datos de entrenamiento o prueba")
	}

	inicioEntrenamiento := time.Now()
	pesos, pesoPositivo, _, _ := entrenar(entrenamiento, *workers, *epocas, *tasa)
	duracion := time.Since(inicioEntrenamiento)
	m := evaluar(prueba, &pesos)

	fmt.Println("\n=== RESULTADOS DE REGRESION LOGISTICA PARALELA ===")
	fmt.Printf("Workers: %d | Epocas: %d | Tiempo de entrenamiento: %s\n", *workers, *epocas, duracion)
	fmt.Printf("Exactitud: %.4f | Precision: %.4f | Recall: %.4f | F1: %.4f\n", m.Exactitud, m.Precision, m.Recall, m.F1)
	fmt.Printf("Matriz de confusion: VP=%d, VN=%d, FP=%d, FN=%d\n", m.VP, m.VN, m.FP, m.FN)
	fmt.Printf("Referencia (predecir siempre clase 0): exactitud %.4f, F1 0.0000\n", float64(m.VN+m.FP)/float64(m.Total))

	modelo := struct {
		Algoritmo    string             `json:"algoritmo"`
		Trabajadores int                `json:"workers"`
		Epocas       int                `json:"epocas"`
		PesoPositivo float64            `json:"peso_clase_1"`
		Descripcion  string             `json:"descripcion"`
		Pesos        [dimension]float64 `json:"pesos"`
		Evaluacion   Metricas           `json:"evaluacion_prueba"`
	}{
		Algoritmo:    "Regresion logistica con gradiente paralelo y codificacion hash categorica",
		Trabajadores: *workers, Epocas: *epocas, PesoPositivo: pesoPositivo,
		Descripcion: "Modelo exploratorio: split hash por OCID, NO validado temporalmente ni apto para alertas reales",
		Pesos:       pesos, Evaluacion: m,
	}
	contenido, err := json.MarshalIndent(modelo, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("data/modelo_logistico.json", contenido, 0644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Modelo guardado: data/modelo_logistico.json")
}
