package bridge

import (
	"strings"
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)


func TestExtractContentFromProto_TemplateMessageWithButtons(t *testing.T) {
	ms := &MessageService{}

	// 1. TemplateMessage with InteractiveMessageTemplate & NativeFlow buttons (modern WhatsApp button templates)
	protoMsg := &waProto.Message{
		TemplateMessage: &waProto.TemplateMessage{
			Format: &waProto.TemplateMessage_InteractiveMessageTemplate{
				InteractiveMessageTemplate: &waProto.InteractiveMessage{
					Header: &waProto.InteractiveMessage_Header{
						Title: proto.String("Código de Acesso"),
					},
					Body: &waProto.InteractiveMessage_Body{
						Text: proto.String("Seu código de confirmação é 789123."),
					},
					Footer: &waProto.InteractiveMessage_Footer{
						Text: proto.String("Válido por 10 minutos."),
					},
					InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{
						NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
							Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
								{
									Name: proto.String("cta_copy"),
									ButtonParamsJSON: proto.String(`{"display_text":"Copiar Código","copy_code":"789123"}`),
								},
								{
									Name: proto.String("cta_url"),
									ButtonParamsJSON: proto.String(`{"display_text":"Acessar Conta","url":"https://example.com/login"}`),
								},
								{
									Name: proto.String("quick_reply"),
									ButtonParamsJSON: proto.String(`{"display_text":"Não fui eu"}`),
								},
							},
						},
					},
				},
			},
		},
	}

	content := ms.ExtractContentFromProto(protoMsg)
	if strings.Contains(content, "[Template Message]") {
		t.Fatalf("expected rich content, but got fallback '[Template Message]'")
	}
	if !strings.Contains(content, "Código de Acesso") {
		t.Errorf("missing header title in: %s", content)
	}
	if !strings.Contains(content, "Seu código de confirmação é 789123.") {
		t.Errorf("missing body text in: %s", content)
	}
	if !strings.Contains(content, "Válido por 10 minutos.") {
		t.Errorf("missing footer text in: %s", content)
	}
	if !strings.Contains(content, "[📋 Copiar Código: 789123]") {
		t.Errorf("missing copy button in: %s", content)
	}
	if !strings.Contains(content, "[🔗 Acessar Conta: https://example.com/login]") {
		t.Errorf("missing url button in: %s", content)
	}
	if !strings.Contains(content, "[🔘 Não fui eu]") {
		t.Errorf("missing quick reply button in: %s", content)
	}
}

func TestExtractContentFromProto_HydratedTemplateButtons(t *testing.T) {
	ms := &MessageService{}

	protoMsg := &waProto.Message{
		TemplateMessage: &waProto.TemplateMessage{
			HydratedTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
				Title: &waProto.TemplateMessage_HydratedFourRowTemplate_HydratedTitleText{
					HydratedTitleText: "Confirmação de Agendamento",
				},
				HydratedContentText: proto.String("Sua consulta está marcada para amanhã às 14:00."),
				HydratedFooterText:  proto.String("Clínica Saúde"),
				HydratedButtons: []*waProto.HydratedTemplateButton{
					{
						HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{
							QuickReplyButton: &waProto.HydratedTemplateButton_HydratedQuickReplyButton{
								DisplayText: proto.String("Confirmar"),
								ID:          proto.String("btn_confirm"),
							},
						},
					},
					{
						HydratedButton: &waProto.HydratedTemplateButton_UrlButton{
							UrlButton: &waProto.HydratedTemplateButton_HydratedURLButton{
								DisplayText: proto.String("Ver Endereço"),
								URL:         proto.String("https://maps.example.com"),
							},
						},
					},
					{
						HydratedButton: &waProto.HydratedTemplateButton_CallButton{
							CallButton: &waProto.HydratedTemplateButton_HydratedCallButton{
								DisplayText: proto.String("Ligar para Clínica"),
								PhoneNumber: proto.String("+5511999999999"),
							},
						},
					},
				},
			},
		},
	}

	content := ms.ExtractContentFromProto(protoMsg)
	if strings.Contains(content, "[Template Message]") {
		t.Fatalf("expected rich content, but got fallback '[Template Message]'")
	}
	if !strings.Contains(content, "Confirmação de Agendamento") {
		t.Errorf("missing title in: %s", content)
	}
	if !strings.Contains(content, "Sua consulta está marcada para amanhã às 14:00.") {
		t.Errorf("missing body in: %s", content)
	}
	if !strings.Contains(content, "Clínica Saúde") {
		t.Errorf("missing footer in: %s", content)
	}
	if !strings.Contains(content, "[🔘 Confirmar]") {
		t.Errorf("missing quick reply button in: %s", content)
	}
	if !strings.Contains(content, "[🔗 Ver Endereço: https://maps.example.com]") {
		t.Errorf("missing url button in: %s", content)
	}
	if !strings.Contains(content, "[📞 Ligar para Clínica: +5511999999999]") {
		t.Errorf("missing call button in: %s", content)
	}
}

func TestExtractContentFromProto_ButtonsMessage(t *testing.T) {
	ms := &MessageService{}

	protoMsg := &waProto.Message{
		ButtonsMessage: &waProto.ButtonsMessage{
			Header: &waProto.ButtonsMessage_Text{
				Text: "Aviso Importante",
			},
			ContentText: proto.String("Você deseja receber novidades por WhatsApp?"),
			FooterText:  proto.String("Você pode alterar isso a qualquer momento."),
			Buttons: []*waProto.ButtonsMessage_Button{
				{
					ButtonID: proto.String("yes"),
					ButtonText: &waProto.ButtonsMessage_Button_ButtonText{
						DisplayText: proto.String("Sim, aceito"),
					},
				},
				{
					ButtonID: proto.String("no"),
					ButtonText: &waProto.ButtonsMessage_Button_ButtonText{
						DisplayText: proto.String("Não, obrigado"),
					},
				},
			},
		},
	}

	content := ms.ExtractContentFromProto(protoMsg)
	if strings.Contains(content, "[Buttons Message]") {
		t.Fatalf("expected rich content, but got fallback '[Buttons Message]'")
	}
	if !strings.Contains(content, "Aviso Importante") {
		t.Errorf("missing header in: %s", content)
	}
	if !strings.Contains(content, "Você deseja receber novidades por WhatsApp?") {
		t.Errorf("missing body in: %s", content)
	}
	if !strings.Contains(content, "[🔘 Sim, aceito]") {
		t.Errorf("missing button 1 in: %s", content)
	}
	if !strings.Contains(content, "[🔘 Não, obrigado]") {
		t.Errorf("missing button 2 in: %s", content)
	}
}

func TestExtractContentFromProto_ButtonResponses(t *testing.T) {
	ms := &MessageService{}

	// TemplateButtonReplyMessage
	msg1 := &waProto.Message{
		TemplateButtonReplyMessage: &waProto.TemplateButtonReplyMessage{
			SelectedDisplayText: proto.String("Opção Selecionada A"),
		},
	}
	if got := ms.ExtractContentFromProto(msg1); got != "Opção Selecionada A" {
		t.Errorf("unexpected TemplateButtonReplyMessage: %s", got)
	}

	// ButtonsResponseMessage
	msg2 := &waProto.Message{
		ButtonsResponseMessage: &waProto.ButtonsResponseMessage{
			Response: &waProto.ButtonsResponseMessage_SelectedDisplayText{
				SelectedDisplayText: "Opção Selecionada B",
			},
		},
	}
	if got := ms.ExtractContentFromProto(msg2); got != "Opção Selecionada B" {
		t.Errorf("unexpected ButtonsResponseMessage: %s", got)
	}

	// InteractiveResponseMessage
	msg3 := &waProto.Message{
		InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
			Body: &waProto.InteractiveResponseMessage_Body{
				Text: proto.String("Resposta Interativa C"),
			},
		},
	}
	if got := ms.ExtractContentFromProto(msg3); got != "Resposta Interativa C" {
		t.Errorf("unexpected InteractiveResponseMessage: %s", got)
	}
}

func TestExtractMediaFromTemplate(t *testing.T) {
	imgMsg := &waProto.ImageMessage{
		Caption: proto.String("Foto anexada"),
		URL:     proto.String("https://mmg.whatsapp.net/d/f/123.enc"),
	}

	protoMsg := &waProto.Message{
		TemplateMessage: &waProto.TemplateMessage{
			Format: &waProto.TemplateMessage_InteractiveMessageTemplate{
				InteractiveMessageTemplate: &waProto.InteractiveMessage{
					Header: &waProto.InteractiveMessage_Header{
						Media: &waProto.InteractiveMessage_Header_ImageMessage{
							ImageMessage: imgMsg,
						},
					},
					Body: &waProto.InteractiveMessage_Body{
						Text: proto.String("Veja nossa promoção!"),
					},
				},
			},
		},
	}

	extractedImg := extractImageMessage(protoMsg)
	if extractedImg == nil {
		t.Fatalf("expected image to be extracted from template message")
	}
	if extractedImg.GetURL() != "https://mmg.whatsapp.net/d/f/123.enc" {
		t.Errorf("unexpected URL: %s", extractedImg.GetURL())
	}
}
