# PC4 - Evidencia del entrenamiento distribuido

## 1. Objetivo
Evaluar el funcionamiento y rendimiento del entrenamiento
distribuido de regresión logística mediante un coordinador
TCP y Workers implementados en Go.

## 2. Arquitectura
El coordinador TCP distribuye rangos de mini-lotes y
los pesos globales del modelo. Cada Worker calcula
gradientes y los devuelve mediante TCP.

El coordinador agrega los resultados y actualiza
los pesos utilizando AdamW.

La implementación actual utiliza particionamiento
lógico: cada Worker carga el dataset completo, pero
procesa solamente los rangos que recibe.

## 3. Configuración experimental
- Entorno: Docker Compose en una computadora.
- Configuraciones: 1, 2 y 4 Workers.
- Goroutines: 2 por Worker.
- Repeticiones: 5 por configuración.
- Registros de entrenamiento: 393181.
- Épocas: 1.
- Tamaño de mini-lote: 8192.
- Mini-lotes procesados: 48.
- Semilla: 42.
- Parámetros del modelo: 13037.

## 4. Resultados

| Workers | Promedio (ms) | Speedup | Eficiencia |
|---------|---------------|---------|------------|
| 1       | 160.734       | 1.00x   | 100.0%     |
| 2       | 191.875       | 0.84x   | 41.9%      |
| 4       | 295.836       | 0.54x   | 13.6%      |

Speedup = tiempo promedio con 1 Worker /
          tiempo promedio con N Workers.

Eficiencia = speedup / cantidad de Workers.

Los tiempos corresponden al entrenamiento medido
por el coordinador, sin incluir el arranque de
contenedores.

Los datos de las 15 ejecuciones se encuentran en:
docs/evidencias/benchmark_pc4.csv

## 5. Consistencia numérica

La comparación de los modelos entrenados con 1 y
4 Workers produjo:

- Pesos en cada modelo: 13037.
- Diferencia absoluta máxima: 4.21884749357559E-15.

Los resultados son numéricamente equivalentes
dentro de la precisión de punto flotante.

## 6. Interpretación

El entrenamiento distribuido se ejecutó
correctamente en las configuraciones evaluadas.

Sin embargo, el aumento de Workers no produjo
una aceleración del entrenamiento en este entorno.

El costo de comunicación y sincronización TCP,
junto con los recursos compartidos de una misma
computadora, puede explicar este comportamiento.
Estos costos no fueron medidos individualmente.

## 7. Limitaciones

- Las pruebas se ejecutaron en una computadora.
- Los Workers son contenedores independientes,
  no máquinas físicas diferentes.
- Se utilizó una época de entrenamiento.
- Los datos se particionan lógicamente, no mediante
  archivos físicos independientes por Worker.
- Los resultados no representan pruebas de carga
  ni rendimiento en infraestructura distribuida real.
