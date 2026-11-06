package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func inspeccionar(nombre string) error {
	archivo, err := os.Open("data/" + nombre)
	if err != nil {
		return err
	}
	defer archivo.Close()

	lector := csv.NewReader(
		bufio.NewReaderSize(archivo, 1024*1024),
	)
	lector.ReuseRecord = true

	// Leer encabezados
	columnas, err := lector.Read()
	if err != nil {
		return err
	}

	// Guardar nombres de columnas
	salida := "columnas_" +
		strings.TrimSuffix(nombre, ".csv") + ".txt"

	err = os.WriteFile(
		salida,
		[]byte(strings.Join(columnas, "\n")),
		0644,
	)
	if err != nil {
		return err
	}

	fmt.Println("\nAnalizando:", nombre)
	fmt.Println("Columnas:", len(columnas))

	inicio := time.Now()
	var total int64

	// Contar registros sin cargarlos en memoria
	for {
		_, err := lector.Read()

		if err == io.EOF {
			break
		}

		if err != nil {
			return fmt.Errorf(
				"error en registro %d: %w",
				total+1, err,
			)
		}

		total++

		if total%1000000 == 0 {
			fmt.Println("Registros:", total)
		}
	}

	fmt.Println("TOTAL:", total)
	fmt.Println("Tiempo:", time.Since(inicio))
	fmt.Println("Encabezados guardados en:", salida)

	return nil
}

func main() {
	archivos := []string{
		"main.csv",
		"tender_items.csv",
	}

	for _, nombre := range archivos {
		if err := inspeccionar(nombre); err != nil {
			fmt.Println("ERROR:", err)
		}
	}
}
