package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"grupo5-prograconcurrente/modelocontrolado"
	"grupo5-prograconcurrente/persistencia"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

//go:embed openapi.json
var especificacionOpenAPI []byte

const paginaSwagger = `<!DOCTYPE html>
<html lang="es">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Swagger UI - API PC4</title>
    <link rel="stylesheet"
          href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        body {
            margin: 0;
            background: #fafafa;
        }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>

    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>

    <script>
        window.onload = function () {
            SwaggerUIBundle({
                url: "/openapi.json",
                dom_id: "#swagger-ui",
                deepLinking: true
            });
        };
    </script>
</body>
</html>`

// Datos enviados por el cliente.
type SolicitudPrediccion struct {
	OCID             string `json:"ocid"`
	Metodo           string `json:"metodo"`
	Categoria        string `json:"categoria"`
	FechaPublicacion string `json:"fecha_publicacion"`
	BuyerID          string `json:"buyer_id"`
	MetodoDetalle    string `json:"metodo_detalle"`
	Titulo           string `json:"titulo"`
	Descripcion      string `json:"descripcion"`
}

// Respuesta de la predicción.
type RespuestaPrediccion struct {
	ID             string  `json:"id,omitempty"`
	OCID           string  `json:"ocid"`
	Probabilidad   float64 `json:"probabilidad"`
	ClasePredicha  uint8   `json:"clase_predicha"`
	Umbral         float64 `json:"umbral"`
	Interpretacion string  `json:"interpretacion"`
	Modelo         string  `json:"modelo"`
}

func responderJSON(w http.ResponseWriter, estado int, datos any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(estado)

	if err := json.NewEncoder(w).Encode(datos); err != nil {
		log.Printf("Error enviando JSON: %v", err)
	}
}

func main() {
	// Cargar el modelo una sola vez al iniciar.
	modelo, err := modelocontrolado.CargarModelo(
		"modelos/modelo_controlado_pc3.json",
	)
	if err != nil {
		log.Fatalf("Error cargando modelo: %v", err)
	}

	// Conectar con MongoDB al iniciar la API.
	ctxConexion, cancelarConexion := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)

	almacen, err := persistencia.Conectar(ctxConexion)
	cancelarConexion()

	if err != nil {
		log.Fatalf("No se pudo conectar con MongoDB: %v", err)
	}

	log.Println("Conexión con MongoDB establecida correctamente")

	defer almacen.Cerrar(context.Background())

	mux := http.NewServeMux()

	// Entregar la especificación OpenAPI.
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(especificacionOpenAPI)
	})

	// Mostrar documentación interactiva Swagger UI.
	mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(paginaSwagger))
	})

	// Comprobar estado de la API.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		responderJSON(w, http.StatusOK, map[string]string{
			"estado":   "ok",
			"servicio": "api-pc4",
		})
	})

	// Realizar predicciones.
	mux.HandleFunc("POST /api/v1/predicciones", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
		defer r.Body.Close()

		var entrada SolicitudPrediccion

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&entrada); err != nil {
			responderJSON(w, http.StatusBadRequest, map[string]string{
				"error": "JSON de entrada inválido: " + err.Error(),
			})
			return
		}

		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			responderJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Se permite un único objeto JSON",
			})
			return
		}

		registro := modelocontrolado.Registro{
			OCID:             entrada.OCID,
			Metodo:           entrada.Metodo,
			Categoria:        entrada.Categoria,
			FechaPublicacion: entrada.FechaPublicacion,
			BuyerID:          entrada.BuyerID,
			MetodoDetalle:    entrada.MetodoDetalle,
			Titulo:           entrada.Titulo,
			Descripcion:      entrada.Descripcion,
		}

		prediccion, err := modelo.Predecir(registro)
		if err != nil {
			responderJSON(w, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}

		interpretacion := "Menor riesgo estimado"
		if prediccion.ClasePredicha == 1 {
			interpretacion = "Mayor riesgo estimado de desierto o cancelación"
		}

		// Preparar los datos que guardaremos en MongoDB.
		documento := persistencia.DocumentoPrediccion{
			OCID:             entrada.OCID,
			Metodo:           entrada.Metodo,
			Categoria:        entrada.Categoria,
			FechaPublicacion: entrada.FechaPublicacion,
			BuyerID:          entrada.BuyerID,
			MetodoDetalle:    entrada.MetodoDetalle,
			Titulo:           entrada.Titulo,
			Descripcion:      entrada.Descripcion,
			Probabilidad:     prediccion.Probabilidad,
			ClasePredicha:    prediccion.ClasePredicha,
			Umbral:           prediccion.Umbral,
			Interpretacion:   interpretacion,
			Modelo:           "Referencia PC3",
			FechaConsulta:    time.Now().UTC(),
		}

		// Guardar la predicción en MongoDB.
		ctxGuardado, cancelarGuardado := context.WithTimeout(
			r.Context(),
			5*time.Second,
		)

		id, err := almacen.Guardar(ctxGuardado, documento)
		cancelarGuardado()

		if err != nil {
			log.Printf("Error guardando predicción: %v", err)

			responderJSON(
				w,
				http.StatusServiceUnavailable,
				map[string]string{
					"error": "No fue posible guardar la predicción",
				},
			)
			return
		}

		// Responder después del guardado exitoso.
		responderJSON(w, http.StatusOK, RespuestaPrediccion{
			ID:             id,
			OCID:           entrada.OCID,
			Probabilidad:   prediccion.Probabilidad,
			ClasePredicha:  prediccion.ClasePredicha,
			Umbral:         prediccion.Umbral,
			Interpretacion: interpretacion,
			Modelo:         "Referencia PC3",
		})

	}) // Cierra el endpoint POST

	// GET: consultar una predicción guardada en MongoDB por su ID.
	mux.HandleFunc("GET /api/v1/predicciones/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		// Comprobar que el identificador tiene formato válido.
		if _, err := bson.ObjectIDFromHex(id); err != nil {
			responderJSON(w, http.StatusBadRequest, map[string]string{
				"error": "El identificador no es válido",
			})
			return
		}

		ctx, cancelar := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancelar()

		documento, err := almacen.BuscarPorID(ctx, id)

		if errors.Is(err, mongo.ErrNoDocuments) {
			responderJSON(w, http.StatusNotFound, map[string]string{
				"error": "Predicción no encontrada",
			})
			return
		}

		if err != nil {
			log.Printf("Error consultando MongoDB: %v", err)
			responderJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "No fue posible consultar la predicción",
			})
			return
		}

		responderJSON(w, http.StatusOK, map[string]any{
			"id":         id,
			"prediccion": documento,
		})
	})

	servidor := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Modelo de referencia PC3 cargado correctamente")
	log.Println("API REST disponible en http://localhost:8080")

	log.Fatal(servidor.ListenAndServe())
}
