import { ApiError } from './client'

// What each API error code means to a customer.
const messages: Record<string, string> = {
  INVALID_CREDENTIALS: 'E-mail ou senha incorretos.',
  MFA_REQUIRED: 'Digite o código do seu aplicativo autenticador.',
  ACCOUNT_LOCKED: 'Muitas tentativas. Tente de novo em 15 minutos.',
  EMAIL_TAKEN: 'Já existe uma conta com este e-mail.',
  RATE_LIMITED: 'Muitas requisições. Aguarde um instante.',
  RETRY_LATER: 'Serviço indisponível no momento. Tente de novo.',
  INSUFFICIENT_FUNDS: 'Saldo insuficiente.',
  LIMIT_EXCEEDED: 'Isso passa do seu limite de transações.',
  ACCOUNT_NOT_ACTIVE: 'A conta não está ativa.',
  RECEIVER_BANK_REJECTED: 'O banco de destino recusou o Pix.',
  KEY_NOT_FOUND: 'Chave Pix não encontrada.',
  NOT_FOUND: 'Não encontrado.',
  TOO_MANY_KEYS: 'Você atingiu o máximo de chaves Pix.',
  KEY_TAKEN: 'Esta chave já está cadastrada.',
  INVALID_MFA_CODE: 'Código do autenticador inválido.',
  TOTP_ALREADY_ENABLED: 'O autenticador já está ativo.',
  IN_PROGRESS: 'Ainda processando. Tente de novo em instantes.',
  QR_CODE_UNAVAILABLE: 'Este QR Code expirou ou já foi pago.',
  REFUND_TOO_LARGE: 'O valor passa do que ainda pode ser devolvido.',
  NOT_REFUNDABLE: 'Este Pix não pode ser devolvido.',
  STEP_UP_REQUIRED: 'Confirme sua senha para continuar.',
  CREDIT_LINE_EXISTS: 'Você já tem um cartão de crédito.',
  PAYMENT_TOO_LARGE: 'O valor passa do que falta pagar.',
  INVOICE_NOT_PAYABLE: 'Esta fatura não pode mais ser paga.',
  BOLETO_ALREADY_PAID: 'Este boleto já foi pago.',
  BOLETO_EXPIRED: 'Este boleto passou do prazo de pagamento.',
  DUPLICATE_PAYMENT: 'Este boleto já está pago ou em pagamento.',
  SETTLEMENT_REJECTED: 'O banco do beneficiário recusou o pagamento.',
  SCHEDULE_AFTER_DUE_DATE: 'Agende até a data de vencimento.',
  UNSUPPORTED_BOLETO: 'Contas de consumo e tributos ainda não são aceitos.',
  INVALID_BOLETO: 'Código de boleto inválido. Confira os números.',
  KYC_REQUIRED: 'Verifique sua identidade antes de abrir a conta.',
  DOCUMENT_MISMATCH: 'O CPF não confere com o seu cadastro.',
  DOCUMENT_IN_USE: 'Este CPF já está cadastrado em outro cliente.',
  IDENTITY_LOCKED: 'Nome, CPF e nascimento não podem mudar depois da verificação.',
  INVALID_PROFILE: 'Confira os dados do cadastro.',
  INVALID_AMOUNT: 'Valor inválido.',
  DESTINATION_NOT_FOUND: 'Conta de destino não encontrada.',
  NOT_CLEARED: 'Esta compra ainda não foi confirmada pela loja.',
  DISPUTE_WINDOW_CLOSED: 'Compras podem ser contestadas em até 90 dias.',
  DISPUTE_ALREADY_OPEN: 'Esta compra já está em contestação.',
  NOTHING_TO_DISPUTE: 'Esta compra já foi totalmente estornada.',
  DISPUTE_NOT_OPEN: 'Esta contestação já foi decidida.',
  CARD_COMPROMISED: 'Este cartão foi bloqueado por segurança. Peça um novo.',
  TOO_MANY_BOXES: 'Você chegou ao máximo de 20 caixinhas.',
  BOX_CLOSED: 'Esta caixinha foi encerrada.',
  INVALID_BUDGET: 'Escolha uma categoria e um valor válido para o orçamento.',
  SESSION_UNKNOWN: 'Entre de novo para gerenciar seus dispositivos.',
  NO_SCHEDULE: 'Esta caixinha não tem depósito programado.',
  NOTHING_TO_YIELD: 'Guarde algum valor na caixinha primeiro.',
  RECEIPT_NOT_FOUND: 'Este código não corresponde a um comprovante do Digital Bank.',
  TOO_MANY_TEMPORARY_CARDS: 'Você pode ter até 3 cartões temporários ao mesmo tempo.',
  TEMPORARY_CARD: 'Cartões temporários não vão para carteiras digitais.',
  LIMIT_BELOW_USED: 'O limite não pode ficar abaixo do que você já usou.',
  LIMIT_REQUEST_IN_REVIEW: 'Você já tem um pedido de limite em análise.',
  INVOICE_NOT_PLANNABLE: 'Só faturas fechadas e não vencidas podem ser parceladas.',
  INVOICE_TOO_SMALL: 'Faturas a partir de R$ 100,00 podem ser parceladas.',
  GATEWAY_DOWN: 'O servidor do banco está desligado. Rode make up no computador.',
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    const known = messages[err.code]
    if (known) return known
    if (err.status === 400 && err.message) return err.message
    return `Não foi possível concluir (${err.code}).`
  }
  return 'Sem conexão com o servidor.'
}
