// Command medir_latencia measures the latency of the prediction API and
// writes the percentiles to stdout and, optionally, to a CSV file.
package main

import (
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"
)

const cuerpoBase = `{"ocid":"ocds-test-%d","metodo":"open","categoria":"services",` +
	`"fecha_publicacion":"2025-06-15","buyer_id":"PE-ENTIDAD-001",` +
	`"metodo_detalle":"LICITACION PUBLICA","titulo":"Servicio de limpieza %d",` +
	`"descripcion":"Contratacion del servicio de limpieza"}`

func percentil(ordenado []time.Duration, p float64) time.Duration {
	if len(ordenado) == 0 {
		return 0
	}
	i := int(float64(len(ordenado)-1) * p)
	return ordenado[i]
}

func main() {
	url := flag.String("url", "http://localhost:8080", "base URL of the API")
	n := flag.Int("n", 500, "requests per scenario")
	distintos := flag.Int("distintos", 50, "distinct inputs (repeats exercise the Redis cache)")
	salida := flag.String("csv", "", "optional CSV output path")
	limite := flag.Duration("limite", 100*time.Millisecond, "latency target for p95")
	flag.Parse()

	cliente := &http.Client{Timeout: 5 * time.Second}

	// Warm up the connection pool.
	if resp, err := cliente.Get(*url + "/health"); err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	} else {
		log.Fatalf("la API no responde: %v", err)
	}

	var duraciones []time.Duration
	var ids []string
	hits := 0

	for i := 0; i < *n; i++ {
		k := i % *distintos
		cuerpo := fmt.Sprintf(cuerpoBase, k, k)

		inicio := time.Now()
		resp, err := cliente.Post(
			*url+"/api/v1/predicciones",
			"application/json",
			bytes.NewBufferString(cuerpo),
		)
		if err != nil {
			log.Fatalf("solicitud %d falló: %v", i, err)
		}
		datos, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		duraciones = append(duraciones, time.Since(inicio))

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("solicitud %d: estado %d: %s", i, resp.StatusCode, datos)
		}
		if resp.Header.Get("X-Cache") == "HIT" {
			hits++
		}
		if len(ids) < 20 {
			if p := bytes.Index(datos, []byte(`"id":"`)); p >= 0 {
				resto := datos[p+6:]
				if q := bytes.IndexByte(resto, '"'); q > 0 {
					ids = append(ids, string(resto[:q]))
				}
			}
		}
	}

	ordenado := append([]time.Duration(nil), duraciones...)
	sort.Slice(ordenado, func(i, j int) bool { return ordenado[i] < ordenado[j] })

	var suma time.Duration
	for _, d := range ordenado {
		suma += d
	}

	p50 := percentil(ordenado, 0.50)
	p95 := percentil(ordenado, 0.95)
	p99 := percentil(ordenado, 0.99)
	max := ordenado[len(ordenado)-1]
	media := suma / time.Duration(len(ordenado))

	fmt.Printf("POST /api/v1/predicciones: n=%d, X-Cache HIT=%d\n", *n, hits)
	fmt.Printf("  media=%v p50=%v p95=%v p99=%v max=%v\n", media, p50, p95, p99, max)

	// GET by id (second read goes through Redis).
	var lecturas []time.Duration
	for pasada := 0; pasada < 2; pasada++ {
		for _, id := range ids {
			inicio := time.Now()
			resp, err := cliente.Get(*url + "/api/v1/predicciones/" + id)
			if err != nil {
				log.Fatalf("GET %s falló: %v", id, err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lecturas = append(lecturas, time.Since(inicio))
		}
	}
	sort.Slice(lecturas, func(i, j int) bool { return lecturas[i] < lecturas[j] })
	if len(lecturas) > 0 {
		fmt.Printf("GET  /api/v1/predicciones/{id}: n=%d p50=%v p95=%v max=%v\n",
			len(lecturas), percentil(lecturas, 0.50), percentil(lecturas, 0.95),
			lecturas[len(lecturas)-1])
	}

	if *salida != "" {
		f, err := os.Create(*salida)
		if err != nil {
			log.Fatal(err)
		}
		w := csv.NewWriter(f)
		_ = w.Write([]string{"solicitud", "latencia_ms"})
		for i, d := range duraciones {
			_ = w.Write([]string{
				strconv.Itoa(i + 1),
				strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 3, 64),
			})
		}
		w.Flush()
		f.Close()
	}

	if p95 > *limite {
		fmt.Printf("RESULTADO: p95 %v supera el límite de %v\n", p95, *limite)
		os.Exit(1)
	}
	fmt.Printf("RESULTADO: p95 %v cumple el límite de %v\n", p95, *limite)
}
