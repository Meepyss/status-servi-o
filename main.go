package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	twilio "github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

const (
	checkInterval = 5 * time.Minute // Intervalo de verificação automática (5 minutos)
)

var (
	alertRecipient string            // Número que receberá os alertas
	sid            string            // Twilio SID
	token          string            // Twilio Auth Token
	services       = []string{"XblGameSave", "OutroServico"} // Serviços a serem monitorados
	lastAlertSent  map[string]time.Time                      // Armazena o último envio de alerta para cada serviço
)

// Verifica se o serviço está rodando
func checkServiceStatus(service string) bool {
	cmd := exec.Command("sc", "query", service)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[ERROR] Falha ao verificar o status do serviço %s: %v", service, err)
		return true // Evita interromper o loop, considera o serviço rodando temporariamente
	}
	result := string(output)
	return strings.Contains(result, "RUNNING")
}

// Envia a mensagem de resposta pelo WhatsApp usando Twilio
func sendWhatsAppMessage(to, message string) {
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: sid,
		Password: token,
	})

	params := &openapi.CreateMessageParams{}
	params.SetTo(to)
	params.SetFrom("whatsapp:+14155238886") // Número Twilio WhatsApp
	params.SetBody(message)

	resp, err := client.Api.CreateMessage(params)
	if err != nil {
		log.Printf("[ERROR] Falha ao enviar mensagem para %s: %v", to, err)
	} else {
		log.Printf("[INFO] Mensagem enviada: SID %s", *resp.Sid)
	}
}

// Validação do número de WhatsApp no formato internacional
func isValidWhatsAppNumber(number string) bool {
	// Expressão regular para números no formato internacional
	re := regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	return re.MatchString(number)
}

// Validação do SID do Twilio
func isValidSID(sid string) bool {
	// SID de conta do Twilio começa com "AC" e tem 34 caracteres
	return strings.HasPrefix(sid, "AC") && len(sid) == 34
}

// Validação do Auth Token do Twilio
func isValidAuthToken(token string) bool {
	// Auth Token tem exatamente 32 caracteres alfanuméricos
	re := regexp.MustCompile(`^[a-zA-Z0-9]{32}$`)
	return re.MatchString(token)
}

// Função que solicita as entradas via terminal
func getUserInputs() {
	reader := bufio.NewReader(os.Stdin)

	// Solicita e valida o número de WhatsApp
	for {
		fmt.Print("Digite o número de WhatsApp (ex: +554799024829): ")
		alertRecipient, _ = reader.ReadString('\n')
		alertRecipient = strings.TrimSpace(alertRecipient)

		if !isValidWhatsAppNumber(alertRecipient) {
			fmt.Println("[ERROR] O número de WhatsApp deve estar no formato internacional, começando com '+'. Exemplo: +554799024829")
		} else {
			break
		}
	}

	// Solicita e valida o SID do Twilio
	for {
		fmt.Print("Digite o SID do Twilio: ")
		sid, _ = reader.ReadString('\n')
		sid = strings.TrimSpace(sid)

		if !isValidSID(sid) {
			fmt.Println("[ERROR] O SID do Twilio deve começar com 'AC' e ter 34 caracteres.")
		} else {
			break
		}
	}

	// Solicita e valida o Auth Token do Twilio
	for {
		fmt.Print("Digite o Auth Token do Twilio: ")
		token, _ = reader.ReadString('\n')
		token = strings.TrimSpace(token)

		if !isValidAuthToken(token) {
			fmt.Println("[ERROR] O Auth Token deve ter 32 caracteres alfanuméricos.")
		} else {
			break
		}
	}

	log.Println("[INFO] Configurações fornecidas com sucesso!")
}

// Processa o webhook e responde com o status do serviço ou permite pausar o envio de alertas
func handleWebhook(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	from := r.FormValue("From")
	msgBody := strings.ToLower(r.FormValue("Body"))

	log.Printf("[INFO] Mensagem recebida de %s: %s\n", from, msgBody)

	// Verifica se a mensagem recebida contém a palavra "status"
	if msgBody == "status" {
		for _, service := range services {
			statusMessage := "*Status*:\nO serviço " + service + " está em execução."
			if !checkServiceStatus(service) {
				statusMessage = "*Alerta*:\nO serviço " + service + " não está em execução."
			}
			sendWhatsAppMessage(from, statusMessage)
		}
	}
}

// Verifica automaticamente o status dos serviços a cada 5 minutos
func autoCheckServiceStatus() {
	for {
		for _, service := range services {
			serviceRunning := checkServiceStatus(service)
			if !serviceRunning && canSendAlert(service) {
				log.Printf("[WARNING] O serviço %s não está ativo. Enviando alerta via WhatsApp...", service)
				sendWhatsAppMessage(alertRecipient, "*Alerta*:\nO serviço "+service+" não está em execução.")
				lastAlertSent[service] = time.Now()
			}
		}
		time.Sleep(checkInterval)
	}
}

// Verifica se podemos enviar um alerta (limita alertas repetidos)
func canSendAlert(service string) bool {
	if lastAlertSent[service].IsZero() || time.Since(lastAlertSent[service]) > 10*time.Minute {
		return true
	}
	return false
}

func main() {
	// Solicita as entradas do usuário via terminal
	getUserInputs()

	// Inicia o monitoramento dos serviços em segundo plano
	lastAlertSent = make(map[string]time.Time)
	go autoCheckServiceStatus()

	// Configura o servidor HTTP para lidar com mensagens recebidas via webhook
	http.HandleFunc("/webhook", handleWebhook)
	log.Println("[INFO] Servidor HTTP iniciado na porta 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
