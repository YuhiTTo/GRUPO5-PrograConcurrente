// Auditoria de etiquetas, estados y textos del dataset controlado.
// Ejecutar DESDE la raiz: go run .\diagnosticos\auditar_etiquetas_texto.go
// Solo lee CSV existentes; crea un resumen agregado en data/auditoria_etiquetas_texto.csv.
// Las etiquetas proceden de estados FINALES; este reporte NO valida disponibilidad temporal.
package main

import (
    "bufio"
    "encoding/csv"
    "fmt"
    "io"
    "log"
    "os"
    "sort"
    "strconv"
    "strings"
    "time"
)

const (
    desierto uint8 = 1 << iota
    nulo
    cancelado
    contratado
    otro
    vacio
)

type procedimiento struct {
    anio int
    clase int
    items int
    completos int
    desfavorables int
    inesperados int
    detalle uint8
}

type resumen struct {
    n, conTitulo, conDesc, iguales, posibleTextoEstado int
    sinItems, inconsistentes, desiertos, nulos, cancelados, contratados, otros, vacios, mixtos int
}

func indice(h []string, nombre string) int {
    for i, x := range h {
        if strings.TrimSpace(strings.TrimPrefix(x, "\ufeff")) == nombre { return i }
    }
    log.Fatalf("No se encontro la columna %q", nombre)
    return -1
}
func normalizar(s string) string {
    s = strings.TrimSpace(strings.ToUpper(s))
    r := strings.NewReplacer("Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U", "Ñ", "N")
    return r.Replace(s)
}
func clasificarDetalle(s string) uint8 {
    s = normalizar(s)
    if s == "" { return vacio }
    var bits uint8
    if strings.Contains(s, "DESIERTO") { bits |= desierto }
    if strings.Contains(s, "NULO") || strings.Contains(s, "NULIDAD") || strings.Contains(s, "ANULAD") { bits |= nulo }
    if strings.Contains(s, "CANCELAD") { bits |= cancelado }
    if strings.Contains(s, "CONTRATADO") || strings.Contains(s, "ADJUDICAD") { bits |= contratado }
    if bits == 0 { bits = otro }
    return bits
}
func pistaEstadoTexto(s string) bool {
    s = normalizar(s)
    for _, palabra := range []string{"DESIERTO", "CANCELAD", "ANULAD", "NULIDAD", "DECLARADO NULO", "CONTRATADO", "ADJUDICAD", "BUENA PRO CONSENTIDA", "SIN EFECTO"} {
        if strings.Contains(s, palabra) { return true }
    }
    return false
}
func bitsReales(x uint8) int {
    n := 0
    for _, v := range []uint8{desierto, nulo, cancelado, contratado, otro} { if x&v != 0 { n++ } }
    return n
}
func nombreMascara(x uint8) string {
    var a []string
    for _, p := range []struct{ v uint8; n string }{
        {desierto, "DESIERTO"}, {nulo, "NULO/ANULADO"}, {cancelado, "CANCELADO"},
        {contratado, "CONTRATADO/ADJUDICADO"}, {otro, "OTRO"}, {vacio, "VACIO"},
    } { if x&p.v != 0 { a = append(a, p.n) } }
    if len(a) == 0 { return "SIN_ITEMS" }
    return strings.Join(a, "+")
}
type contador struct { texto string; n int }
func top(m map[string]int, limite int) []contador {
    a := make([]contador, 0, len(m))
    for k, v := range m { a = append(a, contador{k, v}) }
    sort.Slice(a, func(i, j int) bool {
        if a[i].n == a[j].n { return a[i].texto < a[j].texto }
        return a[i].n > a[j].n
    })
    if len(a) > limite { a = a[:limite] }
    return a
}
func main() {
    inicio := time.Now()
    f, err := os.Open("data/entrenamiento_controlado.csv")
    if err != nil { log.Fatal("Primero prepara data/entrenamiento_controlado.csv: ", err) }
    defer f.Close()
    r := csv.NewReader(bufio.NewReaderSize(f, 1<<20))
    r.ReuseRecord = true
    h, err := r.Read(); if err != nil { log.Fatal(err) }
    kID, kFecha, kY := indice(h, "ocid"), indice(h, "fecha_publicacion"), indice(h, "riesgo")
    kTitulo, kDesc := indice(h, "titulo"), indice(h, "descripcion")
    ids := make(map[string]int, 550000)
    datos := make([]procedimiento, 0, 550000)
    res := map[[2]int]*resumen{}
    for {
        row, e := r.Read(); if e == io.EOF { break }; if e != nil { log.Fatal(e) }
        fecha := strings.TrimSpace(row[kFecha]); if len(fecha) < 4 { continue }
        anio, e := strconv.Atoi(fecha[:4]); if e != nil || anio < 2018 || anio > 2025 { continue }
        y, e := strconv.Atoi(strings.TrimSpace(row[kY])); if e != nil || (y != 0 && y != 1) { continue }
        ocid := strings.TrimSpace(row[kID]); if ocid == "" { continue }
        if _, ya := ids[ocid]; ya { continue }
        ids[ocid] = len(datos)
        datos = append(datos, procedimiento{anio: anio, clase: y})
        k := [2]int{anio, y}
        if res[k] == nil { res[k] = &resumen{} }
        st := res[k]; st.n++
        titulo, desc := strings.TrimSpace(row[kTitulo]), strings.TrimSpace(row[kDesc])
        if titulo != "" { st.conTitulo++ }
        if desc != "" { st.conDesc++ }
        if titulo != "" && desc != "" && strings.EqualFold(titulo, desc) { st.iguales++ }
        if pistaEstadoTexto(titulo) || pistaEstadoTexto(desc) { st.posibleTextoEstado++ }
    }
    _ = f.Close()

    fi, err := os.Open("data/tender_items.csv")
    if err != nil { log.Fatal(err) }
    defer fi.Close()
    ri := csv.NewReader(bufio.NewReaderSize(fi, 1<<20)); ri.ReuseRecord = true
    hi, e := ri.Read(); if e != nil { log.Fatal(e) }
    ki, ks, kd := indice(hi, "main_ocid"), indice(hi, "status"), indice(hi, "statusDetails")
    leidos, relacionados := 0, 0
    detallesClase1 := make(map[string]int)
    for {
        row, e := ri.Read(); if e == io.EOF { break }; if e != nil { log.Fatal(e) }
        leidos++
        pos, ok := ids[strings.TrimSpace(row[ki])]; if !ok { continue }
        relacionados++
        p := &datos[pos]
        p.items++
        estado := normalizar(row[ks])
        switch estado {
        case "COMPLETE": p.completos++
        case "UNSUCCESSFUL", "CANCELLED", "CANCELED": p.desfavorables++
        default: p.inesperados++
        }
        detalle := strings.TrimSpace(row[kd]); p.detalle |= clasificarDetalle(detalle)
        if p.clase == 1 { detallesClase1[detalle]++ }
    }
    _ = fi.Close()
    combinaciones2024 := make(map[string]int)
    for _, p := range datos {
        st := res[[2]int{p.anio, p.clase}]
        if p.items == 0 { st.sinItems++ }
        if p.clase == 1 && p.completos > 0 { st.inconsistentes++ }
        if p.clase == 0 && (p.desfavorables > 0 || p.inesperados > 0) { st.inconsistentes++ }
        if p.detalle&desierto != 0 { st.desiertos++ }
        if p.detalle&nulo != 0 { st.nulos++ }
        if p.detalle&cancelado != 0 { st.cancelados++ }
        if p.detalle&contratado != 0 { st.contratados++ }
        if p.detalle&otro != 0 { st.otros++ }
        if p.detalle&vacio != 0 { st.vacios++ }
        if bitsReales(p.detalle) > 1 { st.mixtos++ }
        if p.anio == 2024 && p.clase == 1 { combinaciones2024[nombreMascara(p.detalle)]++ }
    }
    fmt.Println("=== AUDITORIA DE ETIQUETAS Y TEXTO (2018-2025) ===")
    fmt.Printf("Procedimientos etiquetados en entrenamiento_controlado.csv: %d\n", len(datos))
    fmt.Printf("Items recorridos: %d | Items vinculados al conjunto controlado: %d\n", leidos, relacionados)
    fmt.Printf("%-5s %5s %9s %9s %9s %10s %8s %10s\n", "ANIO", "CLASE", "REGISTROS", "CON_DESC", "DESC=TTL", "SIN_ITEMS", "INCONS.", "TXT_ESTADO")
    for anio := 2018; anio <= 2025; anio++ {
        for y := 0; y <= 1; y++ {
            s := res[[2]int{anio, y}]; if s == nil { continue }
            fmt.Printf("%-5d %5d %9d %9d %9d %10d %8d %10d\n", anio, y, s.n, s.conDesc, s.iguales, s.sinItems, s.inconsistentes, s.posibleTextoEstado)
        }
    }
    fmt.Println("\n=== TIPO DE RESULTADO ENTRE LOS ETIQUETADOS CLASE 1 ===")
    fmt.Printf("%-5s %9s %9s %9s %10s %9s %9s %8s\n", "ANIO", "CLASE1", "DESIERTO", "NULO", "CANCELADO", "OTROS", "VACIOS", "MULTIPLES")
    for anio := 2018; anio <= 2025; anio++ {
        s := res[[2]int{anio, 1}]; if s == nil { continue }
        fmt.Printf("%-5d %9d %9d %9d %10d %9d %9d %8d\n", anio, s.n, s.desiertos, s.nulos, s.cancelados, s.otros, s.vacios, s.mixtos)
    }
    fmt.Println("\n=== DETALLES MAS FRECUENTES (ITEMS DE CLASE 1) ===")
    for _, x := range top(detallesClase1, 15) { fmt.Printf("%8d  %q\n", x.n, x.texto) }
    fmt.Println("\n=== COMBINACIONES DE RESULTADOS POR PROCEDIMIENTO (CLASE 1, 2024) ===")
    for _, x := range top(combinaciones2024, 12) { fmt.Printf("%8d  %s\n", x.n, x.texto) }
    o, err := os.Create("data/auditoria_etiquetas_texto.csv"); if err != nil { log.Fatal(err) }
    w := csv.NewWriter(o)
    cab := []string{"anio", "clase", "registros", "con_titulo", "con_descripcion", "titulo_igual_descripcion", "texto_posible_estado", "sin_items", "estado_inconsistente", "detalle_desierto", "detalle_nulo", "detalle_cancelado", "detalle_contratado", "detalle_otro", "detalle_vacio", "multiples_detalles"}
    if err := w.Write(cab); err != nil { log.Fatal(err) }
    for anio := 2018; anio <= 2025; anio++ {
        for y := 0; y <= 1; y++ {
            s := res[[2]int{anio, y}]; if s == nil { continue }
            a := []string{strconv.Itoa(anio), strconv.Itoa(y)}
            for _, n := range []int{s.n, s.conTitulo, s.conDesc, s.iguales, s.posibleTextoEstado, s.sinItems, s.inconsistentes, s.desiertos, s.nulos, s.cancelados, s.contratados, s.otros, s.vacios, s.mixtos} { a = append(a, strconv.Itoa(n)) }
            if err := w.Write(a); err != nil { log.Fatal(err) }
        }
    }
    w.Flush(); if err := w.Error(); err != nil { log.Fatal(err) }
    if err := o.Close(); err != nil { log.Fatal(err) }
    fmt.Printf("\nReporte agregado: data/auditoria_etiquetas_texto.csv | Tiempo: %v\n", time.Since(inicio))
    fmt.Println("NOTA: un termino detectado o correlacion NO prueba fuga temporal; se requeririan datos historicos por etapa para verificarla.")
}
