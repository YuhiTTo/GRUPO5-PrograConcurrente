# GRUPO5 – Programación Concurrente y Distribuida (CC65)

**Caso 4: Predicción de Contrataciones y Riesgo en Compras Públicas.**

**Pregunta de impacto social:** ¿qué procedimientos de contratación del Estado
tienen alto riesgo de terminar cancelados o desiertos, para que las entidades y
los órganos de control actúen antes de que se pierda tiempo y dinero público?

Todo está escrito en Go 1.27. Los datos van en `data/` (no se versionan).

## Flujo de datos

| Paso | Comando | Entrada | Salida |
|---|---|---|---|
| 1. Inspección | `go run inspeccionar.go` | `main.csv`, `tender_items.csv` | `columnas_*.txt` |
| 2. Análisis de estados | `go run analizar_estados.go` | `tender_items.csv` | consola |
| 3. Análisis por procedimiento | `go run analizar_procesos.go` | `tender_items.csv` | consola |
| 4. Limpieza y etiquetado concurrente | `go run limpiar_concurrente.go -workers 8` | `tender_items.csv` | `data/etiquetas_procesos.csv` |
| 5. Dataset de entrenamiento | `go run preparar_dataset.go` | etiquetas + `main.csv` | `data/entrenamiento_base.csv` |
| 6. Entrenamiento paralelo | `go run ./cmd/entrenar` | `entrenamiento_base.csv` | `data/modelo_logistico.json` |
| 7. Comparación de workers | `go run ./cmd/entrenar -comparar 1,2,4,8,16` | `entrenamiento_base.csv` | `data/comparacion_workers.csv` |

Los archivos de los pasos 1 a 5 están en la raíz y cada uno tiene su propio
`main`, así que se ejecutan de a uno con `go run archivo.go`.

**Etiqueta (`riesgo`):** se calcula a partir de los ítems de cada procedimiento.

- `0`: todos los ítems terminaron `complete`.
- `1`: todos los ítems terminaron `unsuccessful` o `cancelled`.
- Se excluyen los procedimientos mixtos y los que tienen ítems activos, sin
  estado o con un estado desconocido.

## Modelo: regresión logística con entrenamiento paralelo

El paquete `ml/` contiene el modelo y su entrenamiento.

**Características (70 con los datos actuales):** todas se conocen antes de
la adjudicación.

- `log_monto`: logaritmo del monto en PEN, normalizado. Se toma
  `tender_value_amount_PEN` y, si falta y la moneda es PEN, `tender_value_amount`.
- `log_duracion_convocatoria`: días entre el inicio y el fin de `tenderPeriod`.
- Indicadores de monto, fecha y duración disponibles.
- Año normalizado y mes codificado como seno y coseno.
- One-hot de `metodo`, de `categoria` y de `metodo_detalle` (el tipo de
  procedimiento en mayúsculas y sin tildes). Solo tienen columna propia los
  valores con al menos 100 apariciones (flag `-frecuencia`).

No se usa `tender_numberOfTenderers`: un proceso desierto es justamente uno sin
ofertas válidas, así que esa variable revelaría la respuesta.

**Desbalance de clases:** se usan pesos de clase `n / (2·n_clase)`.

**Entrenamiento (paralelismo de datos con sincronización por pasos):**

```
                 ┌─ tareas[0] ─► worker 0 (fragmento 0) ─┐
coordinador ─────┼─ tareas[1] ─► worker 1 (fragmento 1) ─┼─► respuestas ─► coordinador
(pesos w)        └─ tareas[k] ─► worker k (fragmento k) ─┘    (suma en orden fijo,
                    instantánea de w + rango de filas          actualiza w)
```

1. Los datos se dividen en un fragmento (*shard*) por worker. Cada worker es
   una goroutine dueña de su fragmento.
2. **Difusión:** en cada paso el coordinador envía a cada worker, por su propio
   canal, una copia de los pesos y el rango de filas del mini-lote.
3. **Map:** cada worker calcula la suma de gradientes de su rango
   (`CalcularParcial`).
4. **Barrera y reducción:** el coordinador espera todas las respuestas, las
   suma en orden fijo de worker y actualiza los pesos con descenso de gradiente
   con momento y L2.

**Por qué no hay condiciones de carrera:**

- Solo el coordinador escribe los pesos.
- Los workers solo leen copias que nadie modifica después de enviarlas, y una
  matriz de datos que es de solo lectura.
- La codificación concurrente (`ConstruirDatos`) asigna a cada goroutine un
  rango de filas disjunto.

Sumar los parciales en orden fijo hace que el resultado sea reproducible.

El diseño es el patrón *parameter server* que se distribuirá en la PC4:
cada worker pasará a ser un nodo y los canales pasarán a ser conexiones TCP.

### Parámetros de `cmd/entrenar`

| Flag | Por defecto | Descripción |
|---|---|---|
| `-datos` | `data/entrenamiento_base.csv` | CSV de entrenamiento |
| `-modelo` | `data/modelo_logistico.json` | dónde se guarda el modelo |
| `-workers` | número de CPUs | goroutines que calculan gradientes |
| `-epocas` | `100` | pasadas sobre los datos |
| `-lote` | `0` (lote completo) | filas por actualización |
| `-tasa` | `0.5` | tasa de aprendizaje |
| `-momento` | `0.9` | momento |
| `-l2` | `0.0001` | regularización L2 |
| `-prueba` | `0.2` | fracción reservada para prueba |
| `-umbral` | `0.5` | probabilidad mínima para predecir riesgo |
| `-semilla` | `42` | semilla para barajar |
| `-frecuencia` | `100` | apariciones mínimas para que un valor categórico tenga columna |
| `-comparar` | vacío | lista de workers a comparar, por ejemplo `1,2,4,8` |
| `-reporte` | `data/comparacion_workers.csv` | CSV de la comparación |

## Resultados con los datos reales

Datos: OECE, todos los años (corte del 4 de octubre de 2026). Equipo de 16 CPUs.

| Etapa | Resultado |
|---|---|
| Ítems procesados en la limpieza concurrente | 5,145,770 en 1.96 s |
| Procesos etiquetados | 2,461,327 (20.6% cancelados o desiertos) |
| Entrenamiento / prueba | 1,969,062 / 492,265 filas |
| Modelo base, partición aleatoria (lote completo, 100 épocas) | AUC 0.637, sensibilidad 0.736, F1 0.382 |
| Modelo base, prueba con 2025 | AUC 0.545: se degrada porque depende del año y de los montos |

Comparación de workers (lote completo, 100 épocas):

| Workers | Tiempo | Aceleración | Eficiencia |
|---|---|---|---|
| 1 | 20.08 s | 1.00x | 100% |
| 2 | 10.44 s | 1.92x | 96% |
| 4 | 6.22 s | 3.23x | 81% |
| 8 | 4.16 s | 4.82x | 60% |
| 16 | 3.24 s | 6.19x | 39% |

La pérdida y el AUC son idénticos con cualquier número de workers: paralelizar
no cambia el resultado. La eficiencia baja con muchos workers porque el ancho
de banda de memoria y la barrera de cada paso pasan a ser el límite.

## Modelo seleccionado: experimentos controlados con validación temporal

Las carpetas `preparacion/`, `experimentos/` y `diagnosticos/` contienen el
modelo que se reporta en el informe. Sus diferencias con el modelo base:

- **Sin variables con riesgo de fuga:** no usa montos, presupuesto ni número
  de postores, porque `main.csv` es un registro consolidado que puede traer
  valores posteriores a la convocatoria.
- **Variables:** entidad compradora y palabras del título y de la descripción,
  codificadas con *feature hashing* (13,037 dimensiones).
- **Validación temporal:** entrena con 2018–2023, elige la época y el umbral
  con 2024 y prueba con 2025.

```sh
go run ./preparacion/preparar_dataset_controlado.go
go run ./experimentos/entrenar_paralelo_controlado.go -desde 2018 -validacion 2024 \
    -workers 16 -usar-descripcion=true -evaluar-prueba=true
go run ./experimentos/entrenar_random_forest_controlado.go -desde 2019 -validacion 2024 \
    -arboles 60 -workers 16 -usar-descripcion=true
```

| Modelo | Validación | AUC | Precisión | Sensibilidad | F1 |
|---|---|---|---|---|---|
| Regresión logística controlada | 2024 | 0.663 | 0.269 | 0.734 | 0.393 |
| Regresión logística controlada | 2025 | 0.630 | 0.276 | 0.741 | 0.402 |
| Random Forest controlado (60 árboles) | 2024 | 0.653 | 0.248 | 0.916 | 0.391 |

## Informe PC3

El informe del entregable está en `docs/informe/` en Word y PDF. Las figuras
están como SVG en `docs/informe/figuras/`. Antes de entregarlo, completen
la sección, los nombres y códigos de los integrantes, los porcentajes del
reporte de participación y la captura del tablero Kanban.

## Pruebas

```sh
go test -race ./ml                                       # pruebas con detector de carreras
go test ./ml -run '^$' -bench EntrenarParalelo -benchtime 3x   # benchmark con 1M de filas
```

**Qué se verifica:**

- **Gradiente:** el gradiente analítico coincide con la derivada numérica.
- **Equivalencia:** con lote completo, el resultado con N workers es igual al
  de 1 worker.
- **Reproducibilidad:** dos ejecuciones iguales dan exactamente los mismos pesos.
- **Aprendizaje:** el modelo recupera la regla de riesgo de datos sintéticos
  (AUC > 0.75).
- **Codificación:** la versión paralela produce la misma matriz que la
  secuencial.
- **Validaciones:** lectura del CSV, métricas (matriz de confusión, F1, AUC) y
  guardado y carga del modelo.
