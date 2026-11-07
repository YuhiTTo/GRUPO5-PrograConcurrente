package ml

import (
	"fmt"
	"math"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
)

// registrosSinteticos genera procedimientos con una regla de riesgo conocida:
// la contratación directa y los montos altos aumentan la probabilidad de
// cancelación. Sirve para comprobar que el modelo aprende esa regla.
func registrosSinteticos(n int, semilla uint64) []Registro {
	aleatorio := rand.New(rand.NewPCG(semilla, 7))
	metodos := []string{"direct", "limited", "open", "selective"}
	categorias := []string{"goods", "services", "works"}
	tipos := []string{"ADJUDICACION SIMPLIFICADA", "LICITACION PUBLICA", "SUBASTA INVERSA ELECTRONICA"}

	registros := make([]Registro, n)
	for i := range registros {
		r := Registro{
			OCID:      fmt.Sprintf("ocds-%d", i),
			Metodo:    metodos[aleatorio.IntN(len(metodos))],
			Categoria: categorias[aleatorio.IntN(len(categorias))],
			Anio:      2018 + aleatorio.IntN(7),
			Mes:       1 + aleatorio.IntN(12),

			MetodoDetalle: tipos[aleatorio.IntN(len(tipos))],
		}
		if aleatorio.Float64() < 0.9 {
			r.MontoDisponible = true
			r.Monto = math.Exp(10 + 2*aleatorio.NormFloat64())
		}
		if aleatorio.Float64() < 0.7 {
			r.DuracionDisponible = true
			r.Duracion = math.Exp(2 + 0.5*aleatorio.NormFloat64())
		}

		logit := -2.0
		if r.Metodo == "direct" {
			logit += 2.5
		}
		if r.Categoria == "works" {
			logit += 1.0
		}
		if r.MetodoDetalle == "SUBASTA INVERSA ELECTRONICA" {
			logit -= 1.0
		}
		if r.MontoDisponible {
			logit += 0.5 * (math.Log1p(r.Monto) - 10) / 2
		}
		if aleatorio.Float64() < sigmoide(logit) {
			r.Riesgo = 1
		}
		registros[i] = r
	}
	return registros
}

func datosSinteticos(t testing.TB, n int) *Datos {
	t.Helper()
	registros := registrosSinteticos(n, 1)
	return ConstruirDatos(registros, AjustarCodificador(registros, 1), 4)
}

func TestLeerRegistros(t *testing.T) {
	encabezado := "ocid,monto_pen,monto_disponible,metodo,categoria,fecha_publicacion,riesgo,metodo_detalle,duracion_convocatoria_dias\n"

	tests := []struct {
		nombre     string
		csv        string
		esperado   []Registro
		errorTiene string
	}{
		{
			nombre: "fila completa",
			csv:    encabezado + "ocds-1,1500.5,1,open,goods,2021-03-15T10:00:00-05:00,1,LICITACION PUBLICA,12.5\n",
			esperado: []Registro{{
				OCID: "ocds-1", Monto: 1500.5, MontoDisponible: true,
				Metodo: "open", Categoria: "goods", Anio: 2021, Mes: 3, Riesgo: 1,
				MetodoDetalle: "LICITACION PUBLICA", Duracion: 12.5, DuracionDisponible: true,
			}},
		},
		{
			nombre: "monto no disponible y fecha inválida",
			csv:    encabezado + "ocds-2,,0,DESCONOCIDO,works,sin fecha,0,,\n",
			esperado: []Registro{{
				OCID: "ocds-2", Metodo: "DESCONOCIDO", Categoria: "works", Riesgo: 0,
				MetodoDetalle: "DESCONOCIDO",
			}},
		},
		{
			nombre: "monto marcado disponible pero no positivo",
			csv:    encabezado + "ocds-3,-5,1,open,goods,2020-01-01,0,CONCURSO PUBLICO,-3\n",
			esperado: []Registro{{
				OCID: "ocds-3", Metodo: "open", Categoria: "goods", Anio: 2020, Mes: 1,
				MetodoDetalle: "CONCURSO PUBLICO",
			}},
		},
		{
			nombre: "columnas en otro orden",
			csv:    "duracion_convocatoria_dias,riesgo,ocid,metodo_detalle,metodo,categoria,monto_pen,monto_disponible,fecha_publicacion\n0,0,ocds-4,MENOR CUANTIA,,goods,10,1,2019-12-01\n",
			esperado: []Registro{{
				OCID: "ocds-4", Monto: 10, MontoDisponible: true,
				Metodo: "DESCONOCIDO", Categoria: "goods", Anio: 2019, Mes: 12,
				MetodoDetalle: "MENOR CUANTIA", DuracionDisponible: true,
			}},
		},
		{
			nombre:     "falta una columna",
			csv:        "ocid,monto_pen,monto_disponible,metodo,categoria,riesgo\n",
			errorTiene: `falta la columna "fecha_publicacion"`,
		},
		{
			nombre:     "etiqueta inválida",
			csv:        encabezado + "ocds-1,10,1,open,goods,2021-01-01,0,,\nocds-2,10,1,open,goods,2021-01-01,2,,\n",
			errorTiene: `registro 2: riesgo inválido "2"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			registros, err := LeerRegistros(strings.NewReader(tt.csv))

			if tt.errorTiene != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errorTiene) {
					t.Fatalf("error = %v, se esperaba que contenga %q", err, tt.errorTiene)
				}
				return
			}
			if err != nil {
				t.Fatalf("error inesperado: %v", err)
			}
			if len(registros) != len(tt.esperado) {
				t.Fatalf("se leyeron %d registros, se esperaban %d", len(registros), len(tt.esperado))
			}
			for i := range registros {
				if registros[i] != tt.esperado[i] {
					t.Errorf("registro %d = %+v, se esperaba %+v", i, registros[i], tt.esperado[i])
				}
			}
		})
	}
}

func TestCodificador(t *testing.T) {
	entrenamiento := []Registro{
		{
			Monto: math.E - 1, MontoDisponible: true, Metodo: "open", Categoria: "goods", Anio: 2020, Mes: 1,
			MetodoDetalle: "LICITACION PUBLICA", Duracion: math.E - 1, DuracionDisponible: true,
		},
		{
			Monto: math.E*math.E*math.E - 1, MontoDisponible: true, Metodo: "direct", Categoria: "works", Anio: 2022, Mes: 7,
			MetodoDetalle: "CONTRATACION DIRECTA", Duracion: math.E*math.E*math.E - 1, DuracionDisponible: true,
		},
	}
	c := AjustarCodificador(entrenamiento, 1)

	if got := c.NumCaracteristicas(); got != numColumnasNumericas+2+2+2 {
		t.Fatalf("NumCaracteristicas = %d", got)
	}
	nombres := c.NombresCaracteristicas()
	if len(nombres) != c.NumCaracteristicas() {
		t.Fatalf("hay %d nombres para %d características", len(nombres), c.NumCaracteristicas())
	}

	x := make([]float64, c.NumCaracteristicas())

	c.Codificar(entrenamiento[1], x)
	esperado := map[string]float64{
		"log_monto":        1, // log1p = 3, media 2, desviación 1
		"monto_disponible": 1,
		"fecha_disponible": 1,
		"anio":             1, // 2022, media 2021, desviación 1
		"mes_seno":         math.Sin(math.Pi),
		"mes_coseno":       -1,
		"metodo=direct":    1,
		"metodo=open":      0,
		"categoria=goods":  0,
		"categoria=works":  1,

		"log_duracion_convocatoria": 1, // log1p = 3, media 2, desviación 1
		"duracion_disponible":       1,
		"tipo=CONTRATACION DIRECTA": 1,
		"tipo=LICITACION PUBLICA":   0,
	}
	for i, nombre := range nombres {
		if math.Abs(x[i]-esperado[nombre]) > 1e-12 {
			t.Errorf("%s = %v, se esperaba %v", nombre, x[i], esperado[nombre])
		}
	}

	t.Run("valores no vistos y faltantes quedan en cero", func(t *testing.T) {
		c.Codificar(Registro{Metodo: "nuevo", Categoria: "otra", MetodoDetalle: "OTRO TIPO"}, x)
		for i, v := range x {
			if v != 0 {
				t.Errorf("%s = %v, se esperaba 0", nombres[i], v)
			}
		}
	})

	t.Run("los valores raros no tienen columna propia", func(t *testing.T) {
		registros := []Registro{
			{Metodo: "open", Categoria: "goods", MetodoDetalle: "MENOR CUANTIA"},
			{Metodo: "open", Categoria: "goods", MetodoDetalle: "MENOR CUANTIA"},
			{Metodo: "open", Categoria: "works", MetodoDetalle: "CONCURSO PUBLICO"},
		}
		raros := AjustarCodificador(registros, 2)
		if got := strings.Join(raros.NombresCaracteristicas()[numColumnasNumericas:], ","); got != "metodo=open,categoria=goods,tipo=MENOR CUANTIA" {
			t.Errorf("columnas one-hot = %s", got)
		}
	})
}

func TestConstruirDatosParaleloIgualSecuencial(t *testing.T) {
	registros := registrosSinteticos(5000, 3)
	c := AjustarCodificador(registros, 1)

	secuencial := ConstruirDatos(registros, c, 1)
	for _, workers := range []int{2, 7, 16} {
		paralelo := ConstruirDatos(registros, c, workers)
		for i := range secuencial.X {
			if secuencial.X[i] != paralelo.X[i] {
				t.Fatalf("workers=%d: X[%d] = %v, se esperaba %v", workers, i, paralelo.X[i], secuencial.X[i])
			}
		}
		for i := range secuencial.Y {
			if secuencial.Y[i] != paralelo.Y[i] {
				t.Fatalf("workers=%d: Y[%d] distinto", workers, i)
			}
		}
	}
}

// El gradiente analítico debe coincidir con la derivada numérica de la
// pérdida: si esto falla, el modelo aprendería en la dirección equivocada.
func TestCalcularParcialCoincideConGradienteNumerico(t *testing.T) {
	d := datosSinteticos(t, 200)
	pesoClase := PesosDeClase(d)

	m := NuevoModelo(d.P)
	aleatorio := rand.New(rand.NewPCG(5, 5))
	for j := range m.Pesos {
		m.Pesos[j] = aleatorio.NormFloat64() * 0.3
	}
	m.Sesgo = 0.2

	parcial := CalcularParcial(m, d, 0, d.N, pesoClase)
	perdida := func(mod *Modelo) float64 {
		return CalcularParcial(mod, d, 0, d.N, pesoClase).Perdida
	}

	const h = 1e-6
	for j := 0; j <= d.P; j++ {
		mas, menos := m.Clonar(), m.Clonar()
		if j < d.P {
			mas.Pesos[j] += h
			menos.Pesos[j] -= h
		} else {
			mas.Sesgo += h
			menos.Sesgo -= h
		}
		numerico := (perdida(mas) - perdida(menos)) / (2 * h)
		if math.Abs(numerico-parcial.Gradiente[j]) > 1e-4*math.Max(1, math.Abs(numerico)) {
			t.Errorf("gradiente[%d] = %v, numérico = %v", j, parcial.Gradiente[j], numerico)
		}
	}
}

// Con lote completo, el gradiente de cada paso es la suma sobre todas las
// filas sin importar cómo se repartan: el resultado con N workers debe ser
// el mismo que con 1 worker (salvo el orden de las sumas en coma flotante).
func TestEntrenarParaleloIgualSecuencialConLoteCompleto(t *testing.T) {
	d := datosSinteticos(t, 3000)
	cfg := Config{Workers: 1, Epocas: 30, Tasa: 0.5, Momento: 0.9, L2: 1e-4}

	base, err := EntrenarParalelo(d, cfg)
	if err != nil {
		t.Fatal(err)
	}

	for _, workers := range []int{2, 3, 8} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			cfg.Workers = workers
			r, err := EntrenarParalelo(d, cfg)
			if err != nil {
				t.Fatal(err)
			}
			for j := range base.Modelo.Pesos {
				if math.Abs(base.Modelo.Pesos[j]-r.Modelo.Pesos[j]) > 1e-9 {
					t.Errorf("peso %d = %v, secuencial = %v", j, r.Modelo.Pesos[j], base.Modelo.Pesos[j])
				}
			}
			if math.Abs(base.Modelo.Sesgo-r.Modelo.Sesgo) > 1e-9 {
				t.Errorf("sesgo = %v, secuencial = %v", r.Modelo.Sesgo, base.Modelo.Sesgo)
			}
			if r.Pasos != cfg.Epocas {
				t.Errorf("pasos = %d, se esperaba uno por época (%d)", r.Pasos, cfg.Epocas)
			}
		})
	}
}

// La reducción en orden fijo hace que dos ejecuciones iguales den
// exactamente los mismos pesos, aunque las goroutines terminen en otro orden.
func TestEntrenarParaleloEsReproducible(t *testing.T) {
	d := datosSinteticos(t, 4000)
	cfg := Config{Workers: 4, Epocas: 5, TamLote: 256, Tasa: 0.1, Momento: 0.9, L2: 1e-4}

	a, err := EntrenarParalelo(d, cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := EntrenarParalelo(d, cfg)
	if err != nil {
		t.Fatal(err)
	}

	for j := range a.Modelo.Pesos {
		if a.Modelo.Pesos[j] != b.Modelo.Pesos[j] {
			t.Fatalf("peso %d difiere entre ejecuciones: %v vs %v", j, a.Modelo.Pesos[j], b.Modelo.Pesos[j])
		}
	}
	// 1000 filas por worker / 64 filas por worker y paso = 16 pasos por época.
	if a.Pasos != 5*16 {
		t.Errorf("pasos = %d, se esperaban %d", a.Pasos, 5*16)
	}
}

func TestEntrenarParaleloAprendeLaRegla(t *testing.T) {
	registros := registrosSinteticos(40000, 11)
	Barajar(registros, 42)
	entrenamiento, prueba := Dividir(registros, 0.2)
	c := AjustarCodificador(entrenamiento, 1)
	dEntrenamiento := ConstruirDatos(entrenamiento, c, 4)
	dPrueba := ConstruirDatos(prueba, c, 4)

	r, err := EntrenarParalelo(dEntrenamiento, Config{
		Workers: 4, Epocas: 150, Tasa: 0.5, Momento: 0.9, L2: 1e-4,
	})
	if err != nil {
		t.Fatal(err)
	}

	if primera, ultima := r.Perdidas[0], r.Perdidas[len(r.Perdidas)-1]; ultima >= primera {
		t.Errorf("la pérdida no bajó: %v -> %v", primera, ultima)
	}

	met := Evaluar(r.Modelo, dPrueba, 0.5)
	if met.AUC < 0.75 {
		t.Errorf("AUC en prueba = %.3f, se esperaba al menos 0.75", met.AUC)
	}
	if met.VP+met.FP+met.VN+met.FN != dPrueba.N {
		t.Errorf("la matriz de confusión no suma %d filas", dPrueba.N)
	}

	// El modelo debe descubrir que la contratación directa es la señal de
	// riesgo más fuerte de los datos sintéticos.
	nombres := c.NombresCaracteristicas()
	pesos := map[string]float64{}
	for j, nombre := range nombres {
		pesos[nombre] = r.Modelo.Pesos[j]
	}
	if pesos["metodo=direct"] <= pesos["metodo=open"] {
		t.Errorf("peso de metodo=direct (%v) debería superar al de metodo=open (%v)",
			pesos["metodo=direct"], pesos["metodo=open"])
	}
}

func TestEntrenarParaleloRechazaConfigInvalida(t *testing.T) {
	d := datosSinteticos(t, 10)
	valida := Config{Workers: 2, Epocas: 1, Tasa: 0.1}

	tests := []struct {
		nombre  string
		datos   *Datos
		cambiar func(*Config)
		mensaje string
	}{
		{"sin datos", &Datos{}, func(*Config) {}, "no hay datos de entrenamiento"},
		{"sin workers", d, func(c *Config) { c.Workers = 0 }, "debe haber al menos un worker"},
		{"sin épocas", d, func(c *Config) { c.Epocas = 0 }, "debe haber al menos una época"},
		{"lote negativo", d, func(c *Config) { c.TamLote = -1 }, "el tamaño de lote no puede ser negativo"},
		{"tasa cero", d, func(c *Config) { c.Tasa = 0 }, "la tasa de aprendizaje debe ser positiva"},
		{"momento uno", d, func(c *Config) { c.Momento = 1 }, "el momento debe estar en [0, 1)"},
		{"l2 negativo", d, func(c *Config) { c.L2 = -1 }, "la regularización L2 no puede ser negativa"},
	}

	for _, tt := range tests {
		t.Run(tt.nombre, func(t *testing.T) {
			cfg := valida
			tt.cambiar(&cfg)
			r, err := EntrenarParalelo(tt.datos, cfg)
			if err == nil || err.Error() != tt.mensaje {
				t.Fatalf("error = %v, se esperaba %q", err, tt.mensaje)
			}
			if r != nil {
				t.Errorf("se esperaba resultado nil, se obtuvo %+v", r)
			}
		})
	}
}

func TestEntrenarConMasWorkersQueFilas(t *testing.T) {
	d := datosSinteticos(t, 3)
	r, err := EntrenarParalelo(d, Config{Workers: 16, Epocas: 2, Tasa: 0.1})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Perdidas) != 2 {
		t.Errorf("se registraron %d épocas, se esperaban 2", len(r.Perdidas))
	}
}

func TestEvaluar(t *testing.T) {
	// Un solo atributo: el modelo predice riesgo cuando x > 0.
	d := &Datos{
		N: 6, P: 1,
		X: []float64{-3, -2, -1, 1, 2, 3},
		Y: []float64{0, 0, 1, 0, 1, 1},
	}
	m := &Modelo{Pesos: []float64{10}}

	met := Evaluar(m, d, 0.5)

	if met.VP != 2 || met.FP != 1 || met.VN != 2 || met.FN != 1 {
		t.Fatalf("matriz de confusión = VP %d FP %d VN %d FN %d", met.VP, met.FP, met.VN, met.FN)
	}
	casi := func(nombre string, got, want float64) {
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("%s = %v, se esperaba %v", nombre, got, want)
		}
	}
	casi("exactitud", met.Exactitud, 4.0/6)
	casi("precisión", met.Precision, 2.0/3)
	casi("sensibilidad", met.Sensibilidad, 2.0/3)
	casi("F1", met.F1, 2.0/3)
	// Pares positivo-negativo bien ordenados: 8 de 9.
	casi("AUC", met.AUC, 8.0/9)

	t.Run("una sola clase", func(t *testing.T) {
		unaClase := &Datos{N: 2, P: 1, X: []float64{1, 2}, Y: []float64{0, 0}}
		if auc := Evaluar(m, unaClase, 0.5).AUC; auc != 0.5 {
			t.Errorf("AUC = %v, se esperaba 0.5", auc)
		}
	})
}

func TestArchivoModeloIdaYVuelta(t *testing.T) {
	registros := registrosSinteticos(500, 2)
	c := AjustarCodificador(registros, 1)
	d := ConstruirDatos(registros, c, 2)
	r, err := EntrenarParalelo(d, Config{Workers: 2, Epocas: 10, Tasa: 0.5})
	if err != nil {
		t.Fatal(err)
	}

	ruta := filepath.Join(t.TempDir(), "modelo.json")
	original := &ArchivoModelo{Modelo: r.Modelo, Codificador: c, Caracteristicas: c.NombresCaracteristicas()}
	if err := GuardarArchivoModelo(ruta, original); err != nil {
		t.Fatal(err)
	}

	cargado, err := CargarArchivoModelo(ruta)
	if err != nil {
		t.Fatal(err)
	}
	for _, reg := range registros[:50] {
		if a, b := original.Predecir(reg), cargado.Predecir(reg); a != b {
			t.Fatalf("predicción distinta tras recargar: %v vs %v", a, b)
		}
	}
}

// BenchmarkEntrenarParalelo mide la aceleración al aumentar los workers:
//
//	go test ./ml -run '^$' -bench EntrenarParalelo -benchtime 3x
func BenchmarkEntrenarParalelo(b *testing.B) {
	d := datosSinteticos(b, 1_000_000)
	for _, workers := range []int{1, 2, 4, 8, 16} {
		b.Run(fmt.Sprintf("workers=%d", workers), func(b *testing.B) {
			cfg := Config{Workers: workers, Epocas: 5, Tasa: 0.5, Momento: 0.9}
			for b.Loop() {
				if _, err := EntrenarParalelo(d, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
