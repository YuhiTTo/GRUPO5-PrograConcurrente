package main

import (
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pc4 "grupo5-prograconcurrente/modelocontrolado"
)

// Reconstruye la codificación original de PC3 utilizando
// las funciones originales del programa de entrenamiento.
func codificarReferenciaPC3(
	t *testing.T,
	r pc4.Registro,
	usarDescripcion bool,
) Ejemplo {
	t.Helper()

	fecha := strings.TrimSpace(r.FechaPublicacion)

	mes, err := strconv.Atoi(fecha[5:7])
	if err != nil {
		t.Fatalf("Fecha inválida: %v", err)
	}

	familiaDetalle := familia(r.MetodoDetalle)
	metodo := metodoEstable(r.Metodo, r.MetodoDetalle)
	comprador := strings.TrimSpace(r.BuyerID)
	categoria := normal(r.Categoria)

	e := Ejemplo{
		metodo:      uint16(hash(metodo) % nMetodo),
		categoria:   uint16(hash(categoria) % nCategoria),
		comprador:   uint16(hash(comprador) % nComprador),
		detalle:     uint16(hash(familiaDetalle) % nDetalle),
		combinacion: uint16(hash(categoria+"|"+familiaDetalle) % nCombinacion),
		mes:         uint8(mes - 1),
	}

	e.tokens, e.nTokens = extraerTokens(r.Titulo)

	if usarDescripcion {
		e.tokensDesc, e.nTokensDesc =
			extraerTokensDesc(r.Descripcion)
	}

	return e
}

// Compara las características generadas por PC3 y PC4.
func TestCodificacionCoincideConPC3(t *testing.T) {
	registros := []pc4.Registro{
		{
			Metodo:           "OPEN",
			Categoria:        "SERVICIOS",
			FechaPublicacion: "2025-06-15",
			BuyerID:          "PE-ENTIDAD-001",
			MetodoDetalle:    "LICITACION PUBLICA",
			Titulo:           "Adquisicion de equipos informaticos",
			Descripcion:      "Compra de equipos y servicios tecnologicos",
		},
		{
			Metodo:           "DIRECT",
			Categoria:        "BIENES",
			FechaPublicacion: "2024-11-20",
			BuyerID:          "PE-ENTIDAD-002",
			MetodoDetalle:    "CONTRATACION DIRECTA",
			Titulo:           "Compra de materiales medicos",
			Descripcion:      "Adquisicion de materiales para hospitales",
		},
	}

	for i, registro := range registros {
		for _, usarDescripcion := range []bool{false, true} {
			original := codificarReferenciaPC3(
				t, registro, usarDescripcion,
			)

			nuevo, err := pc4.Codificar(
				registro, usarDescripcion,
			)

			if err != nil {
				t.Fatalf("Error PC4: %v", err)
			}

			if original.metodo != nuevo.Metodo ||
				original.categoria != nuevo.Categoria ||
				original.comprador != nuevo.Comprador ||
				original.detalle != nuevo.Detalle ||
				original.combinacion != nuevo.Combinacion ||
				original.mes != nuevo.Mes {
				t.Fatalf(
					"Características diferentes en registro %d",
					i,
				)
			}

			if original.tokens != nuevo.Tokens ||
				original.nTokens != nuevo.NTokens {
				t.Fatalf(
					"Tokens del título diferentes en registro %d",
					i,
				)
			}

			if original.tokensDesc != nuevo.TokensDesc ||
				original.nTokensDesc != nuevo.NTokensDesc {
				t.Fatalf(
					"Tokens de descripción diferentes en registro %d",
					i,
				)
			}
		}
	}
}

// Compara las probabilidades calculadas por ambos motores.
func TestPrediccionCoincideConPC3(t *testing.T) {
	ruta := filepath.Join(
		"..",
		"modelos",
		"modelo_controlado_pc3.json",
	)

	modelo, err := pc4.CargarModelo(ruta)
	if err != nil {
		t.Fatalf("Error cargando modelo: %v", err)
	}

	registro := pc4.Registro{
		Metodo:           "OPEN",
		Categoria:        "SERVICIOS",
		FechaPublicacion: "2025-06-15",
		BuyerID:          "PE-ENTIDAD-001",
		MetodoDetalle:    "LICITACION PUBLICA",
		Titulo:           "Adquisicion de equipos informaticos",
		Descripcion:      "Compra de equipos y servicios tecnologicos",
	}

	original := codificarReferenciaPC3(
		t, registro, modelo.UsaDescripcion,
	)

	nuevo, err := pc4.Codificar(
		registro, modelo.UsaDescripcion,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Motor original de PC3.
	probabilidadPC3 := sigmoid(
		puntaje(&original, modelo.Pesos),
	)

	// Motor nuevo de PC4.
	prediccionPC4, err := modelo.PredecirEjemplo(nuevo)
	if err != nil {
		t.Fatal(err)
	}

	diferencia := math.Abs(
		probabilidadPC3 - prediccionPC4.Probabilidad,
	)

	if diferencia > 1e-12 {
		t.Fatalf(
			"Probabilidades diferentes: PC3=%.15f PC4=%.15f",
			probabilidadPC3,
			prediccionPC4.Probabilidad,
		)
	}

	t.Logf("Probabilidad PC3: %.12f", probabilidadPC3)
	t.Logf("Probabilidad PC4: %.12f", prediccionPC4.Probabilidad)
	t.Logf("Diferencia: %.15f", diferencia)
}
