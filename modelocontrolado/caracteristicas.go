package modelocontrolado

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"unicode"
)

const (
	nMetodo       = 64
	nCategoria    = 32
	nComprador    = 4096
	nDetalle      = 128
	nTexto        = 4096
	nDescripcion  = 4096
	nCombinacion  = 512
	nMes          = 12
	maxTokens     = 12
	maxTokensDesc = 24

	offMes         = 1
	offMetodo      = offMes + nMes
	offCategoria   = offMetodo + nMetodo
	offComprador   = offCategoria + nCategoria
	offDetalle     = offComprador + nComprador
	offTexto       = offDetalle + nDetalle
	offCombinacion = offTexto + nTexto
	offDescripcion = offCombinacion + nCombinacion

	DimensionModelo = offDescripcion + nDescripcion
)

// Registro contiene los datos originales de una contratación.
type Registro struct {
	OCID             string
	Metodo           string
	Categoria        string
	FechaPublicacion string
	BuyerID          string
	MetodoDetalle    string
	Titulo           string
	Descripcion      string
}

// Ejemplo representa una contratación codificada.
type Ejemplo struct {
	Metodo      uint16
	Categoria   uint16
	Comprador   uint16
	Detalle     uint16
	Mes         uint8
	Combinacion uint16

	Tokens      [maxTokens]uint16
	NTokens     uint8
	TokensDesc  [maxTokensDesc]uint16
	NTokensDesc uint8

	Clase uint8
}

var palabrasVacias = map[string]bool{
	"DE": true, "LA": true, "EL": true, "LOS": true,
	"LAS": true, "DEL": true, "AL": true, "PARA": true,
	"POR": true, "CON": true, "EN": true, "Y": true,
	"O": true, "UN": true, "UNA": true, "SE": true,
	"QUE": true, "SU": true, "SUS": true, "A": true,
	"DURANTE": true, "NO": true, "CONTRATACION": true,
	"ADQUISICION": true, "SERVICIO": true, "SERVICIOS": true,
	"BIEN": true, "BIENES": true, "PUBLICA": true,
	"PUBLICO": true, "EJECUCION": true, "ANO": true,
	"ANIO": true, "MEDIANTE": true,
}

// normal reproduce la normalización del modelo original.
func normal(s string) string {
	return strings.NewReplacer(
		"Á", "A",
		"É", "E",
		"Í", "I",
		"Ó", "O",
		"Ú", "U",
		"Ü", "U",
		"Ñ", "N",
	).Replace(strings.ToUpper(strings.TrimSpace(s)))
}

// hash utiliza FNV-1a, igual que el entrenamiento de PC3.
func hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

// familia identifica la familia del procedimiento.
func familia(s string) string {
	v := normal(s)

	switch {
	case strings.Contains(v, "LICITACION"):
		return "LICITACION"
	case strings.Contains(v, "CONCURSO"):
		return "CONCURSO"
	case strings.Contains(v, "ADJUDICACION"):
		return "ADJUDICACION"
	case strings.Contains(v, "SUBASTA"):
		return "SUBASTA"
	case strings.Contains(v, "COMPARACION"):
		return "COMPARACION"
	case strings.Contains(v, "CONTRATACION DIRECTA") ||
		strings.Contains(v, "NO COMPETIT"):
		return "DIRECTA"
	case strings.Contains(v, "REGIMEN"):
		return "REGIMEN"
	case strings.Contains(v, "CONVENIO"):
		return "CONVENIO"
	case strings.Contains(v, "INTERNACIONAL"):
		return "INTERNACIONAL"
	case strings.Contains(v, "MENOR CUANTIA"):
		return "MENOR_CUANTIA"
	case v == "":
		return "SIN_DETALLE"
	default:
		return "OTROS"
	}
}

// metodoEstable normaliza el método de contratación.
func metodoEstable(general, detalle string) string {
	g := normal(general)

	if g == "OPEN" || g == "DIRECT" ||
		g == "SELECTIVE" || g == "LIMITED" {
		return g
	}

	switch familia(detalle) {
	case "DIRECTA":
		return "DIRECT"
	case "LICITACION", "CONCURSO", "ADJUDICACION",
		"SUBASTA", "COMPARACION":
		return "OPEN"
	case "REGIMEN":
		return "REGIMEN"
	case "CONVENIO":
		return "CONVENIO"
	default:
		return "SIN_METODO"
	}
}

// extraerTokens obtiene hasta 12 tokens del título.
func extraerTokens(titulo string) (out [maxTokens]uint16, n uint8) {
	text := normal(titulo)

	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r)
	})

	seen := make(map[uint16]bool, maxTokens)

	for _, w := range words {
		if len([]rune(w)) < 4 ||
			len(w) > 32 ||
			palabrasVacias[w] {
			continue
		}

		id := uint16(hash(w) % nTexto)

		if seen[id] {
			continue
		}

		seen[id] = true
		out[n] = id
		n++

		if n == maxTokens {
			break
		}
	}

	return
}

// extraerTokensDesc obtiene hasta 24 tokens de la descripción.
func extraerTokensDesc(descripcion string) (
	out [maxTokensDesc]uint16,
	n uint8,
) {
	text := normal(descripcion)

	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r)
	})

	seen := make(map[uint16]bool, maxTokensDesc)

	for _, w := range words {
		if len([]rune(w)) < 4 ||
			len(w) > 32 ||
			palabrasVacias[w] {
			continue
		}

		id := uint16(hash(w) % nDescripcion)

		if seen[id] {
			continue
		}

		seen[id] = true
		out[n] = id
		n++

		if n == maxTokensDesc {
			break
		}
	}

	return
}

// Codificar transforma una contratación en las características
// utilizadas por la Regresión Logística de PC3.
func Codificar(r Registro, usarDescripcion bool) (Ejemplo, error) {
	var e Ejemplo

	fecha := strings.TrimSpace(r.FechaPublicacion)

	if len(fecha) < 7 {
		return e, fmt.Errorf("fecha de publicación inválida")
	}

	anio, err := strconv.Atoi(fecha[:4])
	if err != nil || anio < 2004 || anio > 2026 {
		return e, fmt.Errorf("año de publicación inválido")
	}

	mes, err := strconv.Atoi(fecha[5:7])
	if err != nil || mes < 1 || mes > 12 {
		return e, fmt.Errorf("mes de publicación inválido")
	}

	familiaDetalle := familia(r.MetodoDetalle)
	metodo := metodoEstable(r.Metodo, r.MetodoDetalle)
	categoria := normal(r.Categoria)
	comprador := strings.TrimSpace(r.BuyerID)

	e = Ejemplo{
		Metodo:    uint16(hash(metodo) % nMetodo),
		Categoria: uint16(hash(categoria) % nCategoria),
		Comprador: uint16(hash(comprador) % nComprador),
		Detalle:   uint16(hash(familiaDetalle) % nDetalle),
		Combinacion: uint16(
			hash(categoria+"|"+familiaDetalle) % nCombinacion,
		),
		Mes: uint8(mes - 1),
	}

	e.Tokens, e.NTokens = extraerTokens(r.Titulo)

	if usarDescripcion {
		e.TokensDesc, e.NTokensDesc =
			extraerTokensDesc(r.Descripcion)
	}

	return e, nil
}

// CodificarEtiquetado transforma una contratación
// y asigna su etiqueta histórica para el entrenamiento.
func CodificarEtiquetado(
	r Registro,
	clase uint8,
	usarDescripcion bool,
) (Ejemplo, error) {

	if clase > 1 {
		return Ejemplo{}, fmt.Errorf("clase de riesgo inválida")
	}

	e, err := Codificar(r, usarDescripcion)
	if err != nil {
		return Ejemplo{}, err
	}

	e.Clase = clase

	return e, nil
}
