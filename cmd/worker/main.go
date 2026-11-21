package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"grupo5-prograconcurrente/datos"
	"grupo5-prograconcurrente/modelocontrolado"
	"grupo5-prograconcurrente/worker"
)

func main() {

	// Parámetros de ejecución.
	id := flag.String(
		"id",
		"worker-1",
		"Identificador único del worker",
	)

	coordinador := flag.String(
		"coordinador",
		"127.0.0.1:9000",
		"Dirección TCP del coordinador",
	)

	rutaDatos := flag.String(
		"datos",
		"data/entrenamiento_controlado.csv",
		"Ruta del dataset de entrenamiento",
	)

	rutaModelo := flag.String(
		"modelo",
		"modelos/modelo_controlado_pc3.json",
		"Ruta del modelo de referencia de PC3",
	)

	flag.Parse()

	// Validar parámetros.
	if strings.TrimSpace(*id) == "" {
		log.Fatal("El identificador del worker es obligatorio")
	}

	if strings.TrimSpace(*coordinador) == "" {
		log.Fatal("La dirección del coordinador es obligatoria")
	}

	fmt.Println("=== INICIALIZACIÓN DEL WORKER ===")

	fmt.Println("ID:", *id)
	fmt.Println("Coordinador:", *coordinador)

	// 1. Cargar configuración del modelo PC3.
	fmt.Println("\n[1/4] Cargando configuración del modelo...")

	config, err := modelocontrolado.CargarModelo(*rutaModelo)
	if err != nil {
		log.Fatalf("Error cargando modelo: %v", err)
	}

	fmt.Printf(
		"Periodo de entrenamiento: %d-%d\n",
		config.AnioInicioEntrenamiento,
		config.AnioFinEntrenamiento,
	)

	fmt.Println(
		"Usa descripción:",
		config.UsaDescripcion,
	)

	// 2. Cargar el dataset completo.
	fmt.Println("\n[2/4] Cargando dataset...")

	ejemplos, stats, err := datos.CargarEntrenamientoCSV(
		*rutaDatos,
		config.AnioInicioEntrenamiento,
		config.AnioFinEntrenamiento,
		config.UsaDescripcion,
	)

	if err != nil {
		log.Fatalf("Error cargando dataset: %v", err)
	}

	if len(ejemplos) == 0 {
		log.Fatal("El dataset no contiene registros válidos")
	}

	// Comprobar compatibilidad con el modelo de PC3.
	if config.RegistrosEntrenamiento > 0 &&
		len(ejemplos) != config.RegistrosEntrenamiento {

		log.Fatalf(
			"Dataset incompatible: esperado %d registros, recibido %d",
			config.RegistrosEntrenamiento,
			len(ejemplos),
		)
	}

	if stats.Clase0 == 0 || stats.Clase1 == 0 {
		log.Fatal("El dataset debe contener ambas clases")
	}

	fmt.Printf("Registros cargados: %d\n", stats.Registros)
	fmt.Printf("Clase 0: %d\n", stats.Clase0)
	fmt.Printf("Clase 1: %d\n", stats.Clase1)
	fmt.Printf("Filas inválidas: %d\n", stats.Invalidos)

	// 3. Inicializar procesador local.
	fmt.Println("\n[3/4] Inicializando procesador...")

	procesador, err := worker.NuevoProcesador(
		*id,
		ejemplos,
	)

	if err != nil {
		log.Fatalf("Error inicializando worker: %v", err)
	}

	fmt.Println("Procesador inicializado correctamente")
	fmt.Printf("Registros disponibles: %d\n", len(procesador.Datos))

	// 4. Conectar con el coordinador.
	fmt.Println("\n[4/4] Conectando con el coordinador TCP...")
	fmt.Println("Esperando instrucciones...")

	if err := worker.EjecutarTCP(
		*coordinador,
		procesador,
	); err != nil {

		log.Fatalf("Error ejecutando worker: %v", err)
	}

	fmt.Println("\nEl coordinador finalizó el worker.")
	fmt.Println("Worker detenido correctamente.")

	fmt.Println("\n=== FIN DEL WORKER ===")

	_ = fmt.Sprintf
}
