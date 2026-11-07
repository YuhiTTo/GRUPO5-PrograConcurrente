// Comando entrenar: entrena la regresión logística paralela sobre
// data/entrenamiento_base.csv, evalúa con un conjunto de prueba y guarda
// el modelo. Con -comparar mide la aceleración según el número de workers.
//
//	go run ./cmd/entrenar
//	go run ./cmd/entrenar -workers 8 -epocas 200
//	go run ./cmd/entrenar -comparar 1,2,4,8,16
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"grupo5-prograconcurrente/ml"
)

func main() {
	rutaDatos := flag.String("datos", "data/entrenamiento_base.csv", "CSV generado por preparar_dataset.go")
	rutaModelo := flag.String("modelo", "data/modelo_logistico.json", "archivo donde se guarda el modelo")
	workers := flag.Int("workers", runtime.NumCPU(), "goroutines que calculan gradientes")
	epocas := flag.Int("epocas", 100, "pasadas completas sobre los datos")
	tamLote := flag.Int("lote", 0, "filas por actualización; 0 = lote completo")
	tasa := flag.Float64("tasa", 0.5, "tasa de aprendizaje")
	momento := flag.Float64("momento", 0.9, "momento del descenso de gradiente")
	l2 := flag.Float64("l2", 1e-4, "regularización L2")
	fraccionPrueba := flag.Float64("prueba", 0.2, "fracción de datos reservada para prueba")
	umbral := flag.Float64("umbral", 0.5, "probabilidad a partir de la cual se predice riesgo")
	semilla := flag.Uint64("semilla", 42, "semilla para barajar los datos")
	frecuenciaMinima := flag.Int("frecuencia", 100, "apariciones mínimas para que un valor categórico tenga columna propia")
	comparar := flag.String("comparar", "", "lista de workers a comparar, por ejemplo 1,2,4,8")
	rutaComparacion := flag.String("reporte", "data/comparacion_workers.csv", "CSV con la comparación de workers")
	flag.Parse()

	if *fraccionPrueba <= 0 || *fraccionPrueba >= 1 {
		log.Fatal("-prueba debe estar entre 0 y 1")
	}

	cfg := ml.Config{
		Workers: *workers,
		Epocas:  *epocas,
		TamLote: *tamLote,
		Tasa:    *tasa,
		Momento: *momento,
		L2:      *l2,
	}

	// 1. Carga
	inicio := time.Now()
	registros, err := ml.CargarRegistros(*rutaDatos)
	if err != nil {
		log.Fatal(err)
	}
	clase1 := 0
	for _, r := range registros {
		if r.Riesgo == 1 {
			clase1++
		}
	}
	fmt.Println("=== CARGA ===")
	fmt.Println("Registros:", len(registros))
	fmt.Printf("Clase 0 (completado): %d  Clase 1 (cancelado/desierto): %d (%.2f%%)\n",
		len(registros)-clase1, clase1, 100*float64(clase1)/float64(max(1, len(registros))))
	fmt.Println("Tiempo de carga:", time.Since(inicio))

	// 2. Partición y codificación concurrente
	inicio = time.Now()
	ml.Barajar(registros, *semilla)
	entrenamiento, prueba := ml.Dividir(registros, *fraccionPrueba)
	codificador := ml.AjustarCodificador(entrenamiento, *frecuenciaMinima)
	datosEntrenamiento := ml.ConstruirDatos(entrenamiento, codificador, *workers)
	datosPrueba := ml.ConstruirDatos(prueba, codificador, *workers)
	fmt.Println("\n=== CODIFICACIÓN ===")
	fmt.Println("Entrenamiento:", datosEntrenamiento.N, " Prueba:", datosPrueba.N)
	fmt.Println("Características:", datosEntrenamiento.P)
	fmt.Println("Tiempo de codificación:", time.Since(inicio))

	if *comparar != "" {
		compararWorkers(datosEntrenamiento, datosPrueba, cfg, *comparar, *umbral, *rutaComparacion)
		return
	}

	// 3. Entrenamiento paralelo
	fmt.Println("\n=== ENTRENAMIENTO PARALELO ===")
	fmt.Printf("Workers: %d  Épocas: %d  Lote: %s  Tasa: %g  Momento: %g  L2: %g\n",
		cfg.Workers, cfg.Epocas, describirLote(cfg.TamLote), cfg.Tasa, cfg.Momento, cfg.L2)
	cada := max(1, cfg.Epocas/10)
	cfg.AlFinalizarEpoca = func(epoca int, perdida float64) {
		if epoca == 1 || epoca%cada == 0 || epoca == cfg.Epocas {
			fmt.Printf("Época %4d  pérdida %.6f\n", epoca, perdida)
		}
	}
	resultado, err := ml.EntrenarParalelo(datosEntrenamiento, cfg)
	if err != nil {
		log.Fatal(err)
	}
	cfg.AlFinalizarEpoca = nil
	fmt.Println("Actualizaciones de pesos:", resultado.Pasos)
	fmt.Println("Tiempo de entrenamiento:", resultado.Duracion)

	// 4. Evaluación
	metricas := ml.Evaluar(resultado.Modelo, datosPrueba, *umbral)
	imprimirMetricas(metricas)
	imprimirPesos(resultado.Modelo, codificador.NombresCaracteristicas())

	// 5. Persistencia
	err = ml.GuardarArchivoModelo(*rutaModelo, &ml.ArchivoModelo{
		Modelo:          resultado.Modelo,
		Codificador:     codificador,
		Caracteristicas: codificador.NombresCaracteristicas(),
		Config:          cfg,
		MetricasPrueba:  metricas,
		Entrenado:       time.Now(),
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nModelo guardado en:", *rutaModelo)
}

func describirLote(tamLote int) string {
	if tamLote == 0 {
		return "completo"
	}
	return strconv.Itoa(tamLote)
}

func imprimirMetricas(m ml.Metricas) {
	fmt.Println("\n=== EVALUACIÓN (conjunto de prueba) ===")
	fmt.Printf("Umbral: %.2f\n", m.Umbral)
	fmt.Printf("Exactitud: %.4f  Precisión: %.4f  Sensibilidad: %.4f  F1: %.4f\n",
		m.Exactitud, m.Precision, m.Sensibilidad, m.F1)
	fmt.Printf("AUC-ROC: %.4f  Pérdida logarítmica: %.4f\n", m.AUC, m.PerdidaLog)
	fmt.Println("Matriz de confusión:")
	fmt.Printf("                 pred. 0   pred. 1\n")
	fmt.Printf("  real 0 %14d %9d\n", m.VN, m.FP)
	fmt.Printf("  real 1 %14d %9d\n", m.FN, m.VP)
}

// imprimirPesos muestra las características con más influencia. Un peso
// positivo aumenta la probabilidad de cancelación o deserción.
func imprimirPesos(m *ml.Modelo, nombres []string) {
	orden := make([]int, len(nombres))
	for i := range orden {
		orden[i] = i
	}
	sort.Slice(orden, func(a, b int) bool {
		return math.Abs(m.Pesos[orden[a]]) > math.Abs(m.Pesos[orden[b]])
	})

	fmt.Println("\n=== CARACTERÍSTICAS MÁS INFLUYENTES ===")
	for i, j := range orden {
		if i == 10 {
			break
		}
		fmt.Printf("%-40s %+.4f\n", nombres[j], m.Pesos[j])
	}
	fmt.Printf("%-40s %+.4f\n", "(sesgo)", m.Sesgo)
}

// compararWorkers entrena con la misma configuración variando solo el
// número de workers y reporta tiempo, aceleración y eficiencia.
func compararWorkers(entrenamiento, prueba *ml.Datos, cfg ml.Config, lista string, umbral float64, rutaReporte string) {
	var cantidades []int
	for _, parte := range strings.Split(lista, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(parte))
		if err != nil || n < 1 {
			log.Fatalf("valor inválido en -comparar: %q", parte)
		}
		cantidades = append(cantidades, n)
	}

	fmt.Println("\n=== COMPARACIÓN DE WORKERS ===")
	fmt.Printf("Épocas: %d  Lote: %s  CPUs disponibles: %d\n",
		cfg.Epocas, describirLote(cfg.TamLote), runtime.NumCPU())
	fmt.Printf("%8s %14s %12s %11s %10s %8s\n",
		"workers", "tiempo", "aceleración", "eficiencia", "pérdida", "AUC")

	filas := [][]string{{"workers", "tiempo_ms", "aceleracion", "eficiencia", "perdida_final", "auc_prueba", "f1_prueba"}}
	var referencia time.Duration

	for i, w := range cantidades {
		cfg.Workers = w
		resultado, err := ml.EntrenarParalelo(entrenamiento, cfg)
		if err != nil {
			log.Fatal(err)
		}
		if i == 0 {
			referencia = resultado.Duracion
		}
		aceleracion := float64(referencia) / float64(resultado.Duracion)
		eficiencia := aceleracion * float64(cantidades[0]) / float64(w)
		perdida := resultado.Perdidas[len(resultado.Perdidas)-1]
		metricas := ml.Evaluar(resultado.Modelo, prueba, umbral)

		fmt.Printf("%8d %14s %11.2fx %10.0f%% %10.6f %8.4f\n",
			w, resultado.Duracion.Round(time.Millisecond), aceleracion, 100*eficiencia, perdida, metricas.AUC)
		filas = append(filas, []string{
			strconv.Itoa(w),
			strconv.FormatInt(resultado.Duracion.Milliseconds(), 10),
			strconv.FormatFloat(aceleracion, 'f', 3, 64),
			strconv.FormatFloat(eficiencia, 'f', 3, 64),
			strconv.FormatFloat(perdida, 'f', 6, 64),
			strconv.FormatFloat(metricas.AUC, 'f', 4, 64),
			strconv.FormatFloat(metricas.F1, 'f', 4, 64),
		})
	}

	if err := escribirCSV(rutaReporte, filas); err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nLa aceleración se mide respecto de la primera cantidad de workers de la lista.")
	fmt.Println("Reporte guardado en:", rutaReporte)
}

func escribirCSV(ruta string, filas [][]string) error {
	archivo, err := os.Create(ruta)
	if err != nil {
		return err
	}
	escritor := csv.NewWriter(archivo)
	if err := escritor.WriteAll(filas); err != nil {
		archivo.Close()
		return err
	}
	return archivo.Close()
}
