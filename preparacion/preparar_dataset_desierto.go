// Crea un experimento focalizado: CLASE 0 (items COMPLETE) frente a DESIERTO puro (1).
// Fuente: data/entrenamiento_controlado.csv + data/tender_items.csv.
// No introduce variables del desenlace en los predictores.
// NO valida disponibilidad temporal de las caracteristicas.
// Ejecutar desde la raiz: go run .\preparacion\preparar_dataset_desierto.go
package main

import (
    "bufio"
    "encoding/csv"
    "fmt"
    "io"
    "log"
    "os"
    "strconv"
    "strings"
    "time"
)

type registroEstado struct {
    n int
    desiertos int
    invalido bool
}

func col(cab []string, n string) int {
    for i, c := range cab {
        if strings.TrimSpace(strings.TrimPrefix(c, "\ufeff")) == n { return i }
    }
    log.Fatalf("No se encontro columna %q", n)
    return -1
}
func detalleDesierto(s string) bool {
    return strings.EqualFold(strings.TrimSpace(s), "DESIERTO")
}
func main() {
    start := time.Now()
    input := "data/entrenamiento_controlado.csv"
    file, err := os.Open(input); if err != nil { log.Fatal(err) }
    reader := csv.NewReader(bufio.NewReaderSize(file, 1<<20))
    reader.ReuseRecord = true
    header, err := reader.Read(); if err != nil { log.Fatal(err) }
    idCol, classCol := col(header,"ocid"), col(header,"riesgo")
    estados := make(map[string]*registroEstado, 150000)
    var filas int
    for {
        row, e := reader.Read(); if e == io.EOF { break }; if e != nil { log.Fatal(e) }
        filas++
        if row[classCol]=="1" {
            id := strings.TrimSpace(row[idCol])
            if id != "" { estados[id] = &registroEstado{} }
        }
    }
    if err := file.Close(); err != nil { log.Fatal(err) }

    itemsFile, err := os.Open("data/tender_items.csv"); if err != nil { log.Fatal(err) }
    defer itemsFile.Close()
    ir := csv.NewReader(bufio.NewReaderSize(itemsFile, 1<<20)); ir.ReuseRecord = true
    ih, err := ir.Read(); if err != nil { log.Fatal(err) }
    ki, ks, kd := col(ih,"main_ocid"), col(ih,"status"), col(ih,"statusDetails")
    totalItems, relacionados := 0, 0
    for {
        row, e := ir.Read(); if e == io.EOF { break }; if e != nil { log.Fatal(e) }
        totalItems++
        est, ok := estados[strings.TrimSpace(row[ki])]; if !ok { continue }
        relacionados++
        est.n++
        if strings.EqualFold(strings.TrimSpace(row[ks]), "UNSUCCESSFUL") && detalleDesierto(row[kd]) {
            est.desiertos++
        } else {
            est.invalido = true
        }
    }

    file, err = os.Open(input); if err != nil { log.Fatal(err) }
    defer file.Close()
    reader = csv.NewReader(bufio.NewReaderSize(file, 1<<20))
    reader.ReuseRecord = true
    header, err = reader.Read(); if err != nil { log.Fatal(err) }
    idCol, classCol = col(header,"ocid"), col(header,"riesgo")
    yearCol := col(header,"fecha_publicacion")
    output := "data/entrenamiento_desierto.csv"
    fOut, err := os.Create(output); if err != nil { log.Fatal(err) }
    bw := bufio.NewWriterSize(fOut, 1<<20)
    w := csv.NewWriter(bw)
    if err = w.Write(header); err != nil { log.Fatal(err) }
    var class0, class1, excluded, unmatched int
    byYear := map[int][2]int{}
    for {
        row, e := reader.Read(); if e == io.EOF { break }; if e != nil { log.Fatal(e) }
        y := row[classCol]
        include := false
        if y == "0" { include = true; class0++ }
        if y == "1" {
            est := estados[strings.TrimSpace(row[idCol])]
            if est == nil || est.n == 0 { unmatched++ }
            if est != nil && est.n > 0 && !est.invalido && est.n == est.desiertos {
                include = true
                class1++
            } else { excluded++ }
        }
        if !include { continue }
        if len(row[yearCol]) >= 4 {
            yr, e := strconv.Atoi(row[yearCol][:4]); if e == nil {
                v := byYear[yr]; if y == "0" { v[0]++ } else { v[1]++ }; byYear[yr]=v
            }
        }
        if err := w.Write(row); err != nil { log.Fatal(err) }
    }
    w.Flush(); if err := w.Error(); err != nil { log.Fatal(err) }
    if err := bw.Flush(); err != nil { log.Fatal(err) }
    if err := fOut.Close(); err != nil { log.Fatal(err) }
    fmt.Println("=== ETIQUETA FOCALIZADA: CLASE 0 (COMPLETE) vs DESIERTO PURO (1) ===")
    fmt.Printf("Registros controlados: %d | Items recorridos: %d | Items de clase 1 vinculados: %d\n", filas, totalItems, relacionados)
    fmt.Printf("Clase 0: %d | Desierto puro clase 1: %d | Clase 1 excluida: %d | Sin items: %d\n", class0, class1, excluded, unmatched)
    fmt.Printf("%-7s %12s %14s\n", "Anio", "Clase 0", "Desierto puro")
    for yr:=2018; yr<=2025; yr++ { v:=byYear[yr]; fmt.Printf("%-7d %12d %14d\n", yr, v[0], v[1]) }
    fmt.Printf("Generado: %s | Tiempo: %v\n", output, time.Since(start))
    fmt.Println("ADVERTENCIA: No representa todo resultado desfavorable; solo desiertos puros frente a clase 0.")
    fmt.Println("ADVERTENCIA: Los predictores proceden de CSV consolidado; no prueban disponibilidad inicial.")
}
