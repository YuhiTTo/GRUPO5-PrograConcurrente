package main

import (
	_ "embed"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"grupo5-prograconcurrente/modelocontrolado"
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

		responderJSON(w, http.StatusOK, RespuestaPrediccion{
			OCID:           entrada.OCID,
			Probabilidad:   prediccion.Probabilidad,
			ClasePredicha:  prediccion.ClasePredicha,
			Umbral:         prediccion.Umbral,
			Interpretacion: interpretacion,
			Modelo:         "Referencia PC3",
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
