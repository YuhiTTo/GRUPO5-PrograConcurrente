package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

func buscarColumna(columnas []string, nombre string) int {
	for i, columna := range columnas {
		if strings.TrimSpace(columna) == nombre {
			return i
		}
	}
	log.Fatalf("No existe la columna: %s", nombre)
	return -1
}

func montoValido(valor string) (string, string) {
	numero, err := strconv.ParseFloat(
		strings.TrimSpace(valor), 64,
	)

	if err != nil || math.IsNaN(numero) ||
		math.IsInf(numero, 0) || numero <= 0 {
		return "", "0"
	}

	return strconv.FormatFloat(numero, 'f', -1, 64), "1"
}

// elegirMonto usa el monto convertido a soles cuando existe. Si falta,
// usa el monto original siempre que ya esté expresado en soles: el
// 97% de los procedimientos está en PEN, pero solo un tercio trae
// tender_value_amount_PEN.
func elegirMonto(montoPEN, montoOriginal, moneda string) (string, string, bool) {
	if monto, disponible := montoValido(montoPEN); disponible == "1" {
		return monto, disponible, false
	}
	if strings.TrimSpace(moneda) == "PEN" {
		if monto, disponible := montoValido(montoOriginal); disponible == "1" {
			return monto, disponible, true
		}
	}
	return "", "0", false
}

var sinTildes = strings.NewReplacer(
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N",
)

// normalizarTexto unifica variantes como "Licitación Pública" y
// "LICITACION PUBLICA" del tipo de procedimiento.
func normalizarTexto(valor string) string {
	valor = sinTildes.Replace(strings.ToUpper(strings.TrimSpace(valor)))
	return strings.Join(strings.Fields(valor), " ")
}

// duracionDias calcula los días entre el inicio y el fin de la
// convocatoria. Devuelve "" si alguna fecha falta o el plazo es absurdo.
func duracionDias(inicio, fin string) string {
	desde, err := time.Parse(time.RFC3339, strings.TrimSpace(inicio))
	if err != nil {
		return ""
	}
	hasta, err := time.Parse(time.RFC3339, strings.TrimSpace(fin))
	if err != nil {
		return ""
	}
	dias := hasta.Sub(desde).Hours() / 24
	if dias < 0 || dias > 3650 {
		return ""
	}
	return strconv.FormatFloat(dias, 'f', 2, 64)
}

func main() {
	inicio := time.Now()

	// 1. Cargar las etiquetas historicas
	f, err := os.Open("data/etiquetas_procesos.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	r := csv.NewReader(bufio.NewReaderSize(f, 1024*1024))

	cab, err := r.Read()
	if err != nil {
		log.Fatal(err)
	}

	iOCID := buscarColumna(cab, "main_ocid")
	iRiesgo := buscarColumna(cab, "riesgo")

	etiquetas := make(map[string]string)

	for {
		fila, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		ocid := strings.TrimSpace(fila[iOCID])
		riesgo := strings.TrimSpace(fila[iRiesgo])

		if ocid == "" || (riesgo != "0" && riesgo != "1") {
			log.Fatal("Etiqueta invalida")
		}

		etiquetas[ocid] = riesgo
	}

	fmt.Println("Etiquetas cargadas:", len(etiquetas))

	// 2. Leer main.csv
	fMain, err := os.Open("data/main.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer fMain.Close()

	rMain := csv.NewReader(
		bufio.NewReaderSize(fMain, 1024*1024),
	)

	cab, err = rMain.Read()
	if err != nil {
		log.Fatal(err)
	}

	iID := buscarColumna(cab, "ocid")
	iMonto := buscarColumna(cab, "tender_value_amount_PEN")
	iMontoOriginal := buscarColumna(cab, "tender_value_amount")
	iMoneda := buscarColumna(cab, "tender_value_currency")
	iMetodo := buscarColumna(cab, "tender_procurementMethod")
	iMetodoDetalle := buscarColumna(cab, "tender_procurementMethodDetails")
	iCategoria := buscarColumna(cab, "tender_mainProcurementCategory")
	iFecha := buscarColumna(cab, "tender_datePublished")
	iInicioConvocatoria := buscarColumna(cab, "tender_tenderPeriod_startDate")
	iFinConvocatoria := buscarColumna(cab, "tender_tenderPeriod_endDate")

	// 3. Crear el archivo de entrenamiento
	salida, err := os.Create("data/entrenamiento_base.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer salida.Close()

	buffer := bufio.NewWriterSize(salida, 1024*1024)
	w := csv.NewWriter(buffer)

	err = w.Write([]string{
		"ocid",
		"monto_pen",
		"monto_disponible",
		"metodo",
		"categoria",
		"fecha_publicacion",
		"riesgo",
		"metodo_detalle",
		"duracion_convocatoria_dias",
	})
	if err != nil {
		log.Fatal(err)
	}

	procesados := make(map[string]bool)

	var total, generados int64
	var sinEtiqueta, sinMonto, montoRecuperado, sinDuracion int64
	var sinMetodo, sinCategoria int64
	var duplicados, clase0, clase1 int64

	// 4. Procesar cada procedimiento
	for {
		fila, err := rMain.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		total++

		ocid := strings.TrimSpace(fila[iID])

		riesgo, existe := etiquetas[ocid]
		if !existe {
			sinEtiqueta++
			continue
		}

		if procesados[ocid] {
			duplicados++
			continue
		}

		monto, disponible, recuperado := elegirMonto(
			fila[iMonto], fila[iMontoOriginal], fila[iMoneda],
		)
		if disponible == "0" {
			sinMonto++
		}
		if recuperado {
			montoRecuperado++
		}

		metodoDetalle := normalizarTexto(fila[iMetodoDetalle])
		if metodoDetalle == "" {
			metodoDetalle = "DESCONOCIDO"
		}

		duracion := duracionDias(fila[iInicioConvocatoria], fila[iFinConvocatoria])
		if duracion == "" {
			sinDuracion++
		}

		metodo := strings.TrimSpace(fila[iMetodo])
		if metodo == "" {
			metodo = "DESCONOCIDO"
			sinMetodo++
		}

		categoria := strings.TrimSpace(fila[iCategoria])
		if categoria == "" {
			categoria = "DESCONOCIDO"
			sinCategoria++
		}

		fecha := strings.TrimSpace(fila[iFecha])

		err = w.Write([]string{
			ocid,
			monto,
			disponible,
			metodo,
			categoria,
			fecha,
			riesgo,
			metodoDetalle,
			duracion,
		})
		if err != nil {
			log.Fatal(err)
		}

		procesados[ocid] = true
		generados++

		if riesgo == "0" {
			clase0++
		} else {
			clase1++
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatal(err)
	}
	if err := buffer.Flush(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== DATASET PREPARADO ===")
	fmt.Println("Registros main:", total)
	fmt.Println("Sin etiqueta:", sinEtiqueta)
	fmt.Println("Montos recuperados de tender_value_amount (PEN):", montoRecuperado)
	fmt.Println("Montos faltantes o no positivos:", sinMonto)
	fmt.Println("Duraciones de convocatoria faltantes:", sinDuracion)
	fmt.Println("Metodos faltantes:", sinMetodo)
	fmt.Println("Categorias faltantes:", sinCategoria)
	fmt.Println("Duplicados:", duplicados)
	fmt.Println("Registros generados:", generados)
	fmt.Println("Clase 0:", clase0)
	fmt.Println("Clase 1:", clase1)
	fmt.Println("Archivo: data/entrenamiento_base.csv")
	fmt.Println("Tiempo:", time.Since(inicio))
}
