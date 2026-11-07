package ml

import (
	"math"
	"sort"
	"sync"
)

// Posiciones fijas de las características numéricas. Después de ellas
// vienen las columnas one-hot de métodos, categorías y tipos de procedimiento.
const (
	colLogMonto = iota
	colMontoDisponible
	colFechaDisponible
	colAnio
	colMesSeno
	colMesCoseno
	colLogDuracion
	colDuracionDisponible
	numColumnasNumericas
)

// Codificador transforma un Registro en un vector numérico.
// Se ajusta solo con los datos de entrenamiento para no filtrar
// información del conjunto de prueba.
type Codificador struct {
	Metodos          []string `json:"metodos"`
	Categorias       []string `json:"categorias"`
	MetodosDetalle   []string `json:"metodos_detalle"`
	MediaLogMonto    float64  `json:"media_log_monto"`
	DesvLogMonto     float64  `json:"desv_log_monto"`
	MediaAnio        float64  `json:"media_anio"`
	DesvAnio         float64  `json:"desv_anio"`
	MediaLogDuracion float64  `json:"media_log_duracion"`
	DesvLogDuracion  float64  `json:"desv_log_duracion"`

	unaVez          sync.Once
	indiceMetodo    map[string]int
	indiceCategoria map[string]int
	indiceDetalle   map[string]int
}

// AjustarCodificador calcula vocabularios y estadísticas de normalización.
// Solo se crea una columna one-hot para los valores que aparecen al menos
// frecuenciaMinima veces; los valores más raros se codifican como no vistos.
func AjustarCodificador(registros []Registro, frecuenciaMinima int) *Codificador {
	metodos := make(map[string]int)
	categorias := make(map[string]int)
	detalles := make(map[string]int)
	var montos, anios, duraciones estadistico

	for _, r := range registros {
		metodos[r.Metodo]++
		categorias[r.Categoria]++
		detalles[r.MetodoDetalle]++
		if r.MontoDisponible {
			montos.agregar(math.Log1p(r.Monto))
		}
		if r.Anio > 0 {
			anios.agregar(float64(r.Anio))
		}
		if r.DuracionDisponible {
			duraciones.agregar(math.Log1p(r.Duracion))
		}
	}

	c := &Codificador{
		Metodos:        vocabulario(metodos, frecuenciaMinima),
		Categorias:     vocabulario(categorias, frecuenciaMinima),
		MetodosDetalle: vocabulario(detalles, frecuenciaMinima),
	}
	c.MediaLogMonto, c.DesvLogMonto = montos.mediaYDesviacion()
	c.MediaAnio, c.DesvAnio = anios.mediaYDesviacion()
	c.MediaLogDuracion, c.DesvLogDuracion = duraciones.mediaYDesviacion()

	return c
}

// NumCaracteristicas devuelve la longitud del vector codificado.
func (c *Codificador) NumCaracteristicas() int {
	return numColumnasNumericas + len(c.Metodos) + len(c.Categorias) + len(c.MetodosDetalle)
}

// NombresCaracteristicas devuelve el nombre de cada posición del vector,
// útil para interpretar los pesos del modelo.
func (c *Codificador) NombresCaracteristicas() []string {
	nombres := []string{
		"log_monto",
		"monto_disponible",
		"fecha_disponible",
		"anio",
		"mes_seno",
		"mes_coseno",
		"log_duracion_convocatoria",
		"duracion_disponible",
	}
	for _, m := range c.Metodos {
		nombres = append(nombres, "metodo="+m)
	}
	for _, cat := range c.Categorias {
		nombres = append(nombres, "categoria="+cat)
	}
	for _, d := range c.MetodosDetalle {
		nombres = append(nombres, "tipo="+d)
	}
	return nombres
}

// Codificar escribe en dst el vector del registro. dst debe tener
// longitud NumCaracteristicas(). Un valor categórico que no está en el
// vocabulario queda con todas sus columnas en cero.
// Es seguro llamarlo desde varias goroutines a la vez.
func (c *Codificador) Codificar(r Registro, dst []float64) {
	c.unaVez.Do(c.construirIndices)

	for i := range dst {
		dst[i] = 0
	}

	if r.MontoDisponible {
		dst[colLogMonto] = (math.Log1p(r.Monto) - c.MediaLogMonto) / c.DesvLogMonto
		dst[colMontoDisponible] = 1
	}

	if r.Anio > 0 && r.Mes > 0 {
		angulo := 2 * math.Pi * float64(r.Mes-1) / 12
		dst[colFechaDisponible] = 1
		dst[colAnio] = (float64(r.Anio) - c.MediaAnio) / c.DesvAnio
		dst[colMesSeno] = math.Sin(angulo)
		dst[colMesCoseno] = math.Cos(angulo)
	}

	if r.DuracionDisponible {
		dst[colLogDuracion] = (math.Log1p(r.Duracion) - c.MediaLogDuracion) / c.DesvLogDuracion
		dst[colDuracionDisponible] = 1
	}

	desplazamiento := numColumnasNumericas
	if i, existe := c.indiceMetodo[r.Metodo]; existe {
		dst[desplazamiento+i] = 1
	}
	desplazamiento += len(c.Metodos)
	if i, existe := c.indiceCategoria[r.Categoria]; existe {
		dst[desplazamiento+i] = 1
	}
	desplazamiento += len(c.Categorias)
	if i, existe := c.indiceDetalle[r.MetodoDetalle]; existe {
		dst[desplazamiento+i] = 1
	}
}

func (c *Codificador) construirIndices() {
	c.indiceMetodo = indexar(c.Metodos)
	c.indiceCategoria = indexar(c.Categorias)
	c.indiceDetalle = indexar(c.MetodosDetalle)
}

func indexar(valores []string) map[string]int {
	indice := make(map[string]int, len(valores))
	for i, v := range valores {
		indice[v] = i
	}
	return indice
}

// vocabulario devuelve, ordenados, los valores con al menos
// frecuenciaMinima apariciones.
func vocabulario(conteos map[string]int, frecuenciaMinima int) []string {
	valores := make([]string, 0, len(conteos))
	for v, n := range conteos {
		if n >= frecuenciaMinima {
			valores = append(valores, v)
		}
	}
	sort.Strings(valores)
	return valores
}

// estadistico acumula media y varianza con el algoritmo de Welford,
// numéricamente estable para millones de valores.
type estadistico struct {
	n     float64
	media float64
	m2    float64
}

func (e *estadistico) agregar(x float64) {
	e.n++
	delta := x - e.media
	e.media += delta / e.n
	e.m2 += delta * (x - e.media)
}

// mediaYDesviacion devuelve desviación 1 cuando no hay variación,
// para que la normalización nunca divida entre cero.
func (e *estadistico) mediaYDesviacion() (float64, float64) {
	if e.n < 2 {
		return e.media, 1
	}
	desv := math.Sqrt(e.m2 / e.n)
	if desv == 0 {
		return e.media, 1
	}
	return e.media, desv
}
