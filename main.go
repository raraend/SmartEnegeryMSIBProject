package main

import (
    "encoding/csv"
    "fmt"
    "log"
    "strings"
)

func CsvToSlice(data string) (map[string][]string, error) {
	//untuk baca data csv dari string
    reader := csv.NewReader(strings.NewReader(data))
    records, err := reader.ReadAll()
    if err != nil {
        return nil, err
    }
    if len(records) == 0 {
        return nil, fmt.Errorf("CSV is empty")
    } //cek csv ada apa ngga

	//untuk buat map
    result := make(map[string][]string)

    headers := records[0]
    for _, header := range headers {
        result[header] = []string{}
    }

    for _, row := range records[1:] {
        for i, value := range row {
            if i < len(headers) {
                result[headers[i]] = append(result[headers[i]], value)
            }
        }
    } //loopnya

    return result, nil
}

func main() {
    csvData := `Date,Appliance,Energy_Consumption,Room,Status
2022-01-01,Refrigerator,1.2,Kitchen,On
2022-01-01,TV,0.8,Living Room,Off`

    result, err := CsvToSlice(csvData)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result)
}
