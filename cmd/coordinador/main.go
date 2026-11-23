package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"grupo5-prograconcurrente/datos"
	"grupo5-prograconcurrente/distribucion"
	"grupo5-prograconcurrente/modelocontrolado"
)

type conexionWorker struct {
	ID   string
	Conn net.Conn
}

type rango struct {
	Inicio int
	Fin    int
}

// Resultado del entrenamiento distribuido.
// El umbral de clasificación se calibrará posteriormente
// utilizando el conjunto de validación de 2024.
type resultadoPC4 struct {
	Descripcion string `json:"descripcion"`

	AnioInicioEntrenamiento int `json:"anio_inicio_entrenamiento"`
	AnioFinEntrenamiento    int `json:"anio_fin_entrenamiento"`
	AnioValidacion          int `json:"anio_validacion"`

	Workers             int     `json:"workers"`
	EpocasEntrenadas    int     `json:"epocas_entrenadas"`
	EpocasReferenciaPC3 int     `json:"epocas_referencia_pc3"`
	TamanoLote          int     `json:"tamano_lote"`
	Semilla             int64   `json:"semilla"`
	PesoPositivo        float64 `json:"peso_positivo"`
	TasaAdam            float64 `json:"tasa_adam"`

	UsaDescripcion         bool   `json:"usa_descripcion"`
	RegistrosEntrenamiento int    `json:"registros_entrenamiento"`
	RegistrosValidacion    int    `json:"registros_validacion"`
	DuracionEntrenamiento  string `json:"duracion_entrenamiento"`

	Pesos []float64 `json:"pesos"`
}

// Rechaza un worker que no cumpla las condiciones.
func rechazarRegistro(
	conn net.Conn,
	workerID string,
	motivo string,
) {
	if workerID != "" {
		respuesta := distribucion.Mensaje{
			Version:  distribucion.VersionProtocolo,
			Tipo:     distribucion.TipoRegistroOK,
			WorkerID: workerID,
			Aceptado: false,
		}

		_ = distribucion.EnviarMensajeTCP(
			conn,
			respuesta,
			distribucion.TimeoutMensaje,
		)
	}

	_ = conn.Close()
	log.Printf("Registro rechazado: %s", motivo)
}

// Acepta y registra la cantidad solicitada de workers.
func registrarWorkers(
	listener net.Listener,
	cantidad int,
	registrosEsperados int,
) ([]conexionWorker, error) {

	workers := make([]conexionWorker, 0, cantidad)
	registrados := make(map[string]bool)

	fmt.Printf(
		"\nEsperando el registro de %d workers...\n",
		cantidad,
	)

	for len(workers) < cantidad {

		conn, err := listener.Accept()
		if err != nil {
			return nil, fmt.Errorf(
				"error aceptando worker: %w",
				err,
			)
		}

		mensaje, err := distribucion.RecibirMensajeTCP(
			conn,
			distribucion.TimeoutMensaje,
		)

		if err != nil {
			log.Printf("Error recibiendo registro: %v", err)
			conn.Close()
			continue
		}

		if mensaje.Tipo != distribucion.TipoRegistro {
			rechazarRegistro(
				conn,
				mensaje.WorkerID,
				"mensaje de registro esperado",
			)
			continue
		}

		if registrados[mensaje.WorkerID] {
			rechazarRegistro(
				conn,
				mensaje.WorkerID,
				"identificador duplicado",
			)
			continue
		}

		if mensaje.RegistrosDisponibles != registrosEsperados {
			rechazarRegistro(
				conn,
				mensaje.WorkerID,
				fmt.Sprintf(
					"dataset incompatible: esperado %d, recibido %d",
					registrosEsperados,
					mensaje.RegistrosDisponibles,
				),
			)
			continue
		}

		respuesta := distribucion.Mensaje{
			Version:  distribucion.VersionProtocolo,
			Tipo:     distribucion.TipoRegistroOK,
			WorkerID: mensaje.WorkerID,
			Aceptado: true,
		}

		if err := distribucion.EnviarMensajeTCP(
			conn,
			respuesta,
			distribucion.TimeoutMensaje,
		); err != nil {
			log.Printf("Error confirmando registro: %v", err)
			conn.Close()
			continue
		}

		registrados[mensaje.WorkerID] = true

		workers = append(workers, conexionWorker{
			ID:   mensaje.WorkerID,
			Conn: conn,
		})

		fmt.Printf(
			"Worker registrado: %s (%d/%d)\n",
			mensaje.WorkerID,
			len(workers),
			cantidad,
		)
	}

	// Mantener un orden determinista de agregación.
	sort.Slice(workers, func(i, j int) bool {
		return workers[i].ID < workers[j].ID
	})

	return workers, nil
}

// Distribuye los mini-lotes y actualiza el modelo global.
func ejecutarEntrenamiento(
	workers []conexionWorker,
	modelo *modelocontrolado.Modelo,
	optimizador *modelocontrolado.OptimizadorAdamW,
	epocas int,
	tamanoLote int,
) (time.Duration, error) {

	inicioTotal := time.Now()

	totalRegistros := modelo.RegistrosEntrenamiento

	for epoca := 1; epoca <= epocas; epoca++ {

		inicioEpoca := time.Now()

		totalLotes := (totalRegistros + tamanoLote - 1) /
			tamanoLote

		fmt.Printf(
			"\n========== EPOCA %d/%d ==========\n",
			epoca,
			epocas,
		)

		for lote, inicio := 0, 0; inicio < totalRegistros; lote++ {

			fin := inicio + tamanoLote

			if fin > totalRegistros {
				fin = totalRegistros
			}

			jobID := fmt.Sprintf(
				"epoca-%d-lote-%d",
				epoca,
				lote+1,
			)

			rangos := make([]rango, len(workers))

			// 1. Dividir y enviar el trabajo.
			for i, w := range workers {

				desde, hasta, err := datos.RangoWorker(
					inicio,
					fin,
					i,
					len(workers),
				)

				if err != nil {
					return 0, err
				}

				if hasta <= desde {
					return 0, fmt.Errorf(
						"worker %s recibió un segmento vacío",
						w.ID,
					)
				}

				rangos[i] = rango{
					Inicio: desde,
					Fin:    hasta,
				}

				mensaje := distribucion.Mensaje{
					Version:        distribucion.VersionProtocolo,
					Tipo:           distribucion.TipoEntrenar,
					WorkerID:       w.ID,
					JobID:          jobID,
					Epoch:          epoca,
					Batch:          lote,
					Inicio:         desde,
					Fin:            hasta,
					Pesos:          modelo.Pesos,
					PesoPositivo:   modelo.PesoPositivo,
					UsaDescripcion: modelo.UsaDescripcion,
				}

				if err := distribucion.EnviarMensajeTCP(
					w.Conn,
					mensaje,
					distribucion.TimeoutMensaje,
				); err != nil {
					return 0, fmt.Errorf(
						"error enviando trabajo a %s: %w",
						w.ID,
						err,
					)
				}
			}

			// 2. Recibir y agregar los gradientes.
			global := modelocontrolado.GradienteParcial{
				Gradiente: make(
					[]float64,
					modelocontrolado.DimensionModelo,
				),
			}

			for i, w := range workers {

				respuesta, err := distribucion.RecibirMensajeTCP(
					w.Conn,
					distribucion.TimeoutMensaje,
				)

				if err != nil {
					return 0, fmt.Errorf(
						"error recibiendo gradiente de %s: %w",
						w.ID,
						err,
					)
				}

				if respuesta.Tipo == distribucion.TipoError {
					return 0, fmt.Errorf(
						"worker %s: %s",
						w.ID,
						respuesta.Error,
					)
				}

				if respuesta.Tipo != distribucion.TipoGradiente {
					return 0, fmt.Errorf(
						"respuesta inesperada de %s",
						w.ID,
					)
				}

				if respuesta.WorkerID != w.ID ||
					respuesta.JobID != jobID ||
					respuesta.Epoch != epoca ||
					respuesta.Batch != lote {

					return 0, fmt.Errorf(
						"respuesta de trabajo incompatible de %s",
						w.ID,
					)
				}

				esperadas := rangos[i].Fin - rangos[i].Inicio

				if respuesta.Muestras != esperadas {
					return 0, fmt.Errorf(
						"%s: se esperaban %d muestras, llegaron %d",
						w.ID,
						esperadas,
						respuesta.Muestras,
					)
				}

				global.Muestras += respuesta.Muestras
				global.Positivos += respuesta.Positivos
				global.SumaPesos += respuesta.SumaPesos

				for j := 0; j < modelocontrolado.DimensionModelo; j++ {
					global.Gradiente[j] += respuesta.Gradiente[j]
				}
			}

			// 3. Comprobar que se procesó todo el mini-lote.
			if global.Muestras != fin-inicio {
				return 0, fmt.Errorf(
					"mini-lote incompleto: esperado %d, recibido %d",
					fin-inicio,
					global.Muestras,
				)
			}

			// 4. Actualizar pesos globales mediante AdamW.
			if err := optimizador.Actualizar(
				modelo,
				global,
			); err != nil {
				return 0, fmt.Errorf(
					"error actualizando modelo: %w",
					err,
				)
			}

			// Mostrar progreso.
			if lote == 0 ||
				(lote+1)%10 == 0 ||
				lote+1 == totalLotes {

				fmt.Printf(
					"Lote %d/%d | Registros %d/%d | Positivos %d | AdamW %d\n",
					lote+1,
					totalLotes,
					fin,
					totalRegistros,
					global.Positivos,
					optimizador.Iteracion,
				)
			}

			inicio = fin
		}

		fmt.Printf(
			"Época %d finalizada en %v\n",
			epoca,
			time.Since(inicioEpoca),
		)
	}

	return time.Since(inicioTotal), nil
}

// Notifica a los workers que deben detenerse.
func detenerWorkers(workers []conexionWorker) {
	for _, w := range workers {

		mensaje := distribucion.Mensaje{
			Version: distribucion.VersionProtocolo,
			Tipo:    distribucion.TipoDetener,
		}

		if err := distribucion.EnviarMensajeTCP(
			w.Conn,
			mensaje,
			distribucion.TimeoutMensaje,
		); err != nil {
			log.Printf(
				"Error deteniendo %s: %v",
				w.ID,
				err,
			)
		}
	}
}

// Notifica un error a todos los workers.
func notificarError(
	workers []conexionWorker,
	motivo string,
) {
	for _, w := range workers {

		mensaje := distribucion.Mensaje{
			Version:  distribucion.VersionProtocolo,
			Tipo:     distribucion.TipoError,
			WorkerID: w.ID,
			Error:    motivo,
		}

		_ = distribucion.EnviarMensajeTCP(
			w.Conn,
			mensaje,
			distribucion.TimeoutMensaje,
		)
	}
}

// Guarda los pesos y la configuración del entrenamiento PC4.
func guardarResultado(
	ruta string,
	resultado resultadoPC4,
) error {

	if err := os.MkdirAll(
		filepath.Dir(ruta),
		0755,
	); err != nil {
		return err
	}

	contenido, err := json.MarshalIndent(
		resultado,
		"",
		"  ",
	)

	if err != nil {
		return err
	}

	if err := os.WriteFile(
		ruta,
		contenido,
		0644,
	); err != nil {
		return err
	}

	return nil
}

func ejecutar() error {

	direccion := flag.String(
		"addr",
		":9000",
		"Dirección de escucha TCP",
	)

	cantidadWorkers := flag.Int(
		"workers",
		4,
		"Cantidad de workers que deben conectarse",
	)

	epocas := flag.Int(
		"epocas",
		0,
		"Épocas de entrenamiento; 0 utiliza la mejor época de PC3",
	)

	tamanoLote := flag.Int(
		"lote",
		8192,
		"Tamaño del mini-lote",
	)

	rutaModelo := flag.String(
		"modelo",
		"modelos/modelo_controlado_pc3.json",
		"Modelo de referencia de PC3",
	)

	rutaSalida := flag.String(
		"salida",
		"modelos/modelo_pc4_distribuido.json",
		"Archivo de salida del entrenamiento PC4",
	)

	flag.Parse()

	if *cantidadWorkers < 1 {
		return fmt.Errorf("debe existir al menos un worker")
	}

	if *tamanoLote < *cantidadWorkers {
		return fmt.Errorf("el mini-lote debe ser mayor o igual al número de workers")
	}

	// Cargar configuración de PC3.
	config, err := modelocontrolado.CargarModelo(*rutaModelo)
	if err != nil {
		return err
	}

	if *epocas == 0 {
		*epocas = config.MejorEpoca
	}

	if *epocas < 1 {
		return fmt.Errorf("cantidad de épocas inválida")
	}

	if config.RegistrosEntrenamiento <= 0 {
		return fmt.Errorf("cantidad de registros de entrenamiento inválida")
	}

	// Inicializar modelo desde cero.
	modelo := &modelocontrolado.Modelo{
		PesoPositivo:           config.PesoPositivo,
		TasaAdam:               config.TasaAdam,
		UsaDescripcion:         config.UsaDescripcion,
		RegistrosEntrenamiento: config.RegistrosEntrenamiento,
		Pesos: make(
			[]float64,
			modelocontrolado.DimensionModelo,
		),
	}

	// Inicializar optimizador AdamW desde cero.
	optimizador, err := modelocontrolado.NuevoOptimizadorAdamW(
		config.TasaAdam,
	)
	if err != nil {
		return err
	}

	fmt.Println("======================================")
	fmt.Println("       COORDINADOR TCP - PC4")
	fmt.Println("======================================")
	fmt.Println("Workers esperados:", *cantidadWorkers)
	fmt.Println("Registros:", config.RegistrosEntrenamiento)
	fmt.Println("Épocas:", *epocas)
	fmt.Println("Mini-lote:", *tamanoLote)
	fmt.Println("Semilla:", datos.SemillaEntrenamiento)
	fmt.Println("Tasa AdamW:", config.TasaAdam)
	fmt.Println("Peso positivo:", config.PesoPositivo)
	fmt.Println("Descripción:", config.UsaDescripcion)

	// Iniciar servidor TCP.
	listener, err := distribucion.EscucharTCP(*direccion)
	if err != nil {
		return err
	}
	defer listener.Close()

	fmt.Println("\nServidor TCP escuchando en:", *direccion)

	// Registrar todos los workers.
	workers, err := registrarWorkers(
		listener,
		*cantidadWorkers,
		config.RegistrosEntrenamiento,
	)
	if err != nil {
		return err
	}

	defer func() {
		for _, w := range workers {
			_ = w.Conn.Close()
		}
	}()

	fmt.Println("\nTodos los workers están registrados.")

	// Entrenar.
	duracion, err := ejecutarEntrenamiento(
		workers,
		modelo,
		optimizador,
		*epocas,
		*tamanoLote,
	)

	if err != nil {
		notificarError(workers, err.Error())
		return err
	}

	// Detener workers al finalizar correctamente.
	detenerWorkers(workers)

	// Guardar resultado experimental.
	resultado := resultadoPC4{
		Descripcion: "Modelo PC4 entrenado mediante distribución TCP. El umbral debe calibrarse con validación antes de utilizarlo como clasificador.",

		AnioInicioEntrenamiento: config.AnioInicioEntrenamiento,
		AnioFinEntrenamiento:    config.AnioFinEntrenamiento,
		AnioValidacion:          config.AnioValidacion,

		Workers:             *cantidadWorkers,
		EpocasEntrenadas:    *epocas,
		EpocasReferenciaPC3: config.MejorEpoca,
		TamanoLote:          *tamanoLote,
		Semilla:             datos.SemillaEntrenamiento,
		PesoPositivo:        config.PesoPositivo,
		TasaAdam:            config.TasaAdam,

		UsaDescripcion:         config.UsaDescripcion,
		RegistrosEntrenamiento: config.RegistrosEntrenamiento,
		RegistrosValidacion:    config.RegistrosValidacion,
		DuracionEntrenamiento:  duracion.String(),

		Pesos: modelo.Pesos,
	}

	if err := guardarResultado(
		*rutaSalida,
		resultado,
	); err != nil {
		return fmt.Errorf(
			"error guardando modelo: %w",
			err,
		)
	}

	fmt.Println("\n======================================")
	fmt.Println("     ENTRENAMIENTO FINALIZADO")
	fmt.Println("======================================")
	fmt.Println("Workers:", *cantidadWorkers)
	fmt.Println("Épocas:", *epocas)
	fmt.Println("Actualizaciones AdamW:", optimizador.Iteracion)
	fmt.Println("Tiempo:", duracion)
	fmt.Println("Modelo guardado:", *rutaSalida)

	return nil
}

func main() {
	if err := ejecutar(); err != nil {
		log.Fatal(err)
	}
}
