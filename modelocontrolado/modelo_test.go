package modelocontrolado

import (
	"math"
	"path/filepath"
	"testing"
)

func cargarModeloPrueba(t *testing.T) *Modelo {
	t.Helper()

	ruta := filepath.Join("..", "modelos", "modelo_controlado_pc3.json")

	modelo, err := CargarModelo(ruta)
	if err != nil {
		t.Fatalf("Error cargando modelo: %v", err)
	}

	return modelo
}

// Comprueba que los pesos y metadatos del modelo PC3
// se carguen correctamente.
func TestCargarModeloPC3(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	if len(modelo.Pesos) != DimensionModelo {
		t.Fatalf(
			"Dimensiones incorrectas: esperado %d, obtenido %d",
			DimensionModelo,
			len(modelo.Pesos),
		)
	}

	if modelo.AnioInicioEntrenamiento != 2018 {
		t.Errorf("Año inicial incorrecto")
	}

	if modelo.AnioFinEntrenamiento != 2023 {
		t.Errorf("Año final incorrecto")
	}

	if modelo.AnioValidacion != 2024 {
		t.Errorf("Año de validación incorrecto")
	}

	if math.Abs(modelo.Umbral-0.22) > 1e-9 {
		t.Errorf("Umbral incorrecto: %f", modelo.Umbral)
	}

	if !modelo.UsaDescripcion {
		t.Error("El modelo debería utilizar descripción")
	}
}

// Comprueba que se pueda predecir sin conocer
// previamente la etiqueta de riesgo.
func TestPredecirSinEtiqueta(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	registro := Registro{
		Metodo:           "OPEN",
		Categoria:        "SERVICIOS",
		FechaPublicacion: "2025-06-15",
		BuyerID:          "PE-ENTIDAD-001",
		MetodoDetalle:    "LICITACION PUBLICA",
		Titulo:           "Adquisicion de equipos informaticos",
		Descripcion:      "Compra de equipos y servicios tecnologicos",
	}

	prediccion, err := modelo.Predecir(registro)

	if err != nil {
		t.Fatalf("Error realizando predicción: %v", err)
	}

	if math.IsNaN(prediccion.Probabilidad) ||
		math.IsInf(prediccion.Probabilidad, 0) {
		t.Fatal("La probabilidad no es un número válido")
	}

	if prediccion.Probabilidad < 0 ||
		prediccion.Probabilidad > 1 {
		t.Fatalf(
			"Probabilidad fuera de rango: %f",
			prediccion.Probabilidad,
		)
	}

	if math.Abs(prediccion.Umbral-0.22) > 1e-9 {
		t.Errorf("Se utilizó un umbral incorrecto")
	}

	esperada := uint8(0)
	if prediccion.Probabilidad >= prediccion.Umbral {
		esperada = 1
	}

	if prediccion.ClasePredicha != esperada {
		t.Errorf("La clase predicha no coincide con el umbral")
	}
}

// Comprueba que una misma contratación produzca
// siempre la misma predicción.
func TestPrediccionDeterminista(t *testing.T) {
	modelo := cargarModeloPrueba(t)

	registro := Registro{
		Metodo:           "OPEN",
		Categoria:        "SERVICIOS",
		FechaPublicacion: "2025-06-15",
		BuyerID:          "PE-ENTIDAD-001",
		MetodoDetalle:    "LICITACION PUBLICA",
		Titulo:           "Adquisicion de equipos informaticos",
		Descripcion:      "Compra de equipos y servicios tecnologicos",
	}

	p1, err := modelo.Predecir(registro)
	if err != nil {
		t.Fatal(err)
	}

	p2, err := modelo.Predecir(registro)
	if err != nil {
		t.Fatal(err)
	}

	if p1.Probabilidad != p2.Probabilidad {
		t.Error("Las predicciones no son deterministas")
	}

	if p1.ClasePredicha != p2.ClasePredicha {
		t.Error("Las clases predichas son diferentes")
	}
}
