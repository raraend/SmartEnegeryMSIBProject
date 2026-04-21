package main

import (
     "bytes"
    "encoding/csv"
    "encoding/json"
    "fmt"
    "html/template"
    "io"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"

    "github.com/joho/godotenv"
)

type Inputs struct {
    Table map[string][]string `json:"table"`
    Query string              `json:"query"`
}

type Response struct {
    Answer      string   `json:"answer"`
    Coordinates [][]int  `json:"coordinates"`
    Cells       []string `json:"cells"`
    Aggregator  string   `json:"aggregator"`
}

type AIModelConnector struct {
    Client *http.Client
}

type AskRequest struct {
	Query string `json:"query"`
}

type AskResponse struct {
	Answer          string   `json:"answer"`
	Aggregator      string   `json:"aggregator"`
	Recommendations []string `json:"recommendations,omitempty"`
	Error           string   `json:"error,omitempty"`
}

var csvTable map[string][]string

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

func (connector *AIModelConnector) ConnectAIModel(payload Inputs, token string) (Response, error) {
	apiURL := "https://router.huggingface.co/hf-inference/models/google/tapas-base-finetuned-wtq" //url hggfce

	requestBody := map[string]interface{}{
		"inputs": payload,
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return Response{}, fmt.Errorf("failed to marshal payload: %w", err)
	}
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return Response{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := connector.Client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{}, fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}


	var result Response
	err = json.Unmarshal(body, &result)
	if err != nil {
		return Response{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return result, nil

}


func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, nil)
}


func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	file, _, err := r.FormFile("csv")
	if err != nil {
		http.Error(w, `{"error":"Gagal membaca file"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()
	csvBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, `{"error":"Gagal membaca isi file"}`, http.StatusInternalServerError)
		return
	}
	table, err := CsvToSlice(string(csvBytes))
	if err != nil {
		http.Error(w, `{"error":"Format CSV tidak valid"}`, http.StatusBadRequest)
		return
	}
	csvTable = table
	columns := []string{}
	for k := range table {
		columns = append(columns, k)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"columns": columns,
		"rows":    len(table[columns[0]]),
	})
}

func handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if csvTable == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AskResponse{Error: "Upload file CSV terlebih dahulu!"})
		return
	}
	var req AskRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil || req.Query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AskResponse{Error: "Query tidak boleh kosong"})
		return
	}
	connector := &AIModelConnector{Client: &http.Client{}}
	payload := Inputs{
		Table: csvTable,
		Query: req.Query,
	}
	result, err := connector.ConnectAIModel(payload, os.Getenv("HUGGINGFACE_TOKEN"))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AskResponse{Error: "Gagal menghubungi AI: " + err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(AskResponse{
		Answer:          processAnswer(result),
		Aggregator:      result.Aggregator,
		Recommendations: generateRecommendations(csvTable),
	})
}

func processAnswer(result Response) string {
	if result.Aggregator == "SUM" || result.Aggregator == "AVG" || result.Aggregator == "AVERAGE" {
		total := 0.0
		count := 0
		for _, cell := range result.Cells {
			val, err := strconv.ParseFloat(strings.TrimSpace(cell), 64)
			if err == nil {
				total += val
				count++
			}
		}
		if count == 0 {
			return result.Answer
		}
		if result.Aggregator == "SUM" {
			return fmt.Sprintf("%.2f kWh", total)
		}
		if result.Aggregator == "AVG" || result.Aggregator == "AVERAGE" {
			return fmt.Sprintf("%.2f kWh", total/float64(count))
		}
	}
	if result.Aggregator == "COUNT" {
		return fmt.Sprintf("%d items", len(result.Cells))
	}
	return result.Answer
}

func generateRecommendations(table map[string][]string) []string {
	recommendations := []string{}
	energyCol := table["Energy_Consumption"]
	applianceCol := table["Appliance"]
	statusCol := table["Status"]
	roomCol := table["Room"]
	applianceEnergy := make(map[string]float64)
	for i := 0; i < len(energyCol); i++ {
		if i >= len(applianceCol) {
			break
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(energyCol[i]), 64)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(applianceCol[i])
		applianceEnergy[name] += val
	}

	for appliance, total := range applianceEnergy {
		if total > 10.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("⚠️  %s mengonsumsi %.2f kWh — sangat tinggi, segera periksa atau matikan.", appliance, total))
		} else if total > 5.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("💡 %s mengonsumsi %.2f kWh — pertimbangkan untuk membatasi penggunaannya.", appliance, total))
		}
	}
	countOn := 0
	onAppliances := []string{}
	for i := 0; i < len(statusCol); i++ {
		if i >= len(applianceCol) {
			break
		}
		if strings.EqualFold(strings.TrimSpace(statusCol[i]), "on") {
			countOn++
			onAppliances = append(onAppliances, strings.TrimSpace(applianceCol[i]))
		}
	}

	if countOn > 5 {
		recommendations = append(recommendations,
			fmt.Sprintf("🔴 Ada %d appliance menyala sekaligus (%s) — terlalu banyak, matikan yang tidak dipakai.",
				countOn, strings.Join(onAppliances, ", ")))
	} else if countOn > 3 {
		recommendations = append(recommendations,
			fmt.Sprintf("🟡 Ada %d appliance menyala (%s) — pertimbangkan mematikan beberapa.",
				countOn, strings.Join(onAppliances, ", ")))
	}
	roomEnergy := make(map[string]float64)
	for i := 0; i < len(energyCol); i++ {
		if i >= len(roomCol) {
			break
		}
		val, err := strconv.ParseFloat(strings.TrimSpace(energyCol[i]), 64)
		if err != nil {
			continue
		}
		room := strings.TrimSpace(roomCol[i])
		roomEnergy[room] += val
	}
	for room, total := range roomEnergy {
		if total > 8.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("🏠 Ruangan %s mengonsumsi %.2f kWh — lakukan audit penggunaan energi di ruangan ini.", room, total))
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "✅ Konsumsi energi dalam batas normal. Tidak ada tindakan yang diperlukan saat ini.")
	}

	return recommendations
}


func main() {
   err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/upload", handleUpload)
	http.HandleFunc("/ask", handleAsk)
	port := ":8080"
	fmt.Println("Server berjalan di http://localhost" + port)
	log.Fatal(http.ListenAndServe(port, nil))
}
