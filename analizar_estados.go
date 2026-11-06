package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

func mostrarResultados(titulo string, datos map[string]int64) {
	type resultado struct {
		estado   string
		cantidad int64
	}

	var lista []resultado
	for estado, cantidad := range datos {
		lista = append(lista, resultado{estado, cantidad})
	}

	sort.Slice(lista, func(i, j int) bool {
		return lista[i].cantidad > lista[j].cantidad
	})

	fmt.Println("\n", titulo)
	for i, r := range lista {
		if i >= 20 {
			break
		}
		fmt.Printf("%-40s %d\n", r.estado, r.cantidad)
	}
	fmt.Println("Estados diferentes contabilizados:", len(datos))
}

func main() {
	inicio := time.Now()

	archivo, err := os.Open("data/tender_items.csv")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer archivo.Close()

	lector := csv.NewReader(
		bufio.NewReaderSize(archivo, 1024*1024),
	)
	lector.ReuseRecord = true

	columnas, err := lector.Read()
	if err != nil {
		fmt.Println("Error al leer columnas:", err)
		return
	}

	indiceEstado := -1
	indiceDetalle := -1
	indiceOcid := -1

	for i, nombre := range columnas {
		switch nombre {
		case "status":
			indiceEstado = i
		case "statusDetails":
			indiceDetalle = i
		case "main_ocid":
			indiceOcid = i
		}
	}

	if indiceEstado < 0 || indiceDetalle < 0 || indiceOcid < 0 {
		fmt.Println("Faltan columnas requeridas")
		return
	}

	estados := make(map[string]int64)
	detalles := make(map[string]int64)

	var total int64
	var sinOcid int64

	for {
		fila, err := lector.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error de lectura:", err)
			return
		}

		if len(fila) != len(columnas) {
			fmt.Println("Registro con columnas incompletas")
			return
		}

		estado := strings.TrimSpace(fila[indiceEstado])
		detalle := strings.TrimSpace(fila[indiceDetalle])

		if estado == "" {
			estado = "(VACIO)"
		}
		if detalle == "" {
			detalle = "(VACIO)"
		}

		estados[estado]++

		// Limitar el número de categorías almacenadas
		if _, existe := detalles[detalle]; existe || len(detalles) < 1000 {
			detalles[detalle]++
		} else {
			detalles["(OTROS VALORES)"]++
		}

		if strings.TrimSpace(fila[indiceOcid]) == "" {
			sinOcid++
		}

		total++
	}

	fmt.Println("TOTAL DE REGISTROS:", total)
	fmt.Println("Registros sin main_ocid:", sinOcid)

	mostrarResultados("ESTADOS (status)", estados)
	mostrarResultados("DETALLES (statusDetails)", detalles)

	fmt.Println("\nTiempo:", time.Since(inicio))
}
