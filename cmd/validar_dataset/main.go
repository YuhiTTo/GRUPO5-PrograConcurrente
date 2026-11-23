package main

import (
	"flag"
	"fmt"
	"log"

	"grupo5-prograconcurrente/datos"
	"grupo5-prograconcurrente/modelocontrolado"
)

func main() {
	rutaCSV := flag.String(
		"csv",
		"data/entrenamiento_controlado.csv",
		"Ruta del dataset",
	)

	rutaModelo := flag.String(
		"modelo",
		"modelos/modelo_controlado_pc3.json",
		"Ruta del modelo PC3",
	)

	flag.Parse()

	// Cargar configuración y pesos del modelo PC3.
	modelo, err := modelocontrolado.CargarModelo(*rutaModelo)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== CONFIGURACION DEL MODELO PC3 ===")
	fmt.Printf("Periodo: %d-%d\n",
		modelo.AnioInicioEntrenamiento,
		modelo.AnioFinEntrenamiento,
	)
	fmt.Printf("Validacion: %d\n", modelo.AnioValidacion)
	fmt.Printf("Registros esperados: %d\n",
		modelo.RegistrosEntrenamiento,
	)
	fmt.Printf("Descripcion activada: %t\n",
		modelo.UsaDescripcion,
	)

	// Cargar exclusivamente los datos de entrenamiento.
	registros, stats, err := datos.CargarEntrenamientoCSV(
		*rutaCSV,
		modelo.AnioInicioEntrenamiento,
		modelo.AnioFinEntrenamiento,
		modelo.UsaDescripcion,
	)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n=== VALIDACION DEL DATASET REAL ===")
	fmt.Printf("Filas leidas: %d\n", stats.FilasLeidas)
	fmt.Printf("Fuera del periodo: %d\n", stats.FueraPeriodo)
	fmt.Printf("Registros invalidos: %d\n", stats.Invalidos)
	fmt.Printf("Registros cargados: %d\n", len(registros))

	fmt.Println("\n=== DISTRIBUCION DE CLASES ===")
	fmt.Printf("Clase 0: %d\n", stats.Clase0)
	fmt.Printf("Clase 1: %d\n", stats.Clase1)

	fmt.Println("\n=== VALORES FALTANTES ===")
	fmt.Printf("Metodo vacio: %d\n", stats.MetodoVacio)
	fmt.Printf("Descripcion vacia: %d\n",
		stats.DescripcionVacia,
	)

	fmt.Println("\n=== COMPROBACION ===")

	if len(registros) != modelo.RegistrosEntrenamiento {
		log.Fatalf(
			"ERROR: se esperaban %d registros y se cargaron %d",
			modelo.RegistrosEntrenamiento,
			len(registros),
		)
	}

	if stats.Clase0+stats.Clase1 != len(registros) {
		log.Fatal("ERROR: la distribucion de clases no coincide")
	}

	fmt.Println("CORRECTO: dataset compatible con el modelo PC3.")
}
