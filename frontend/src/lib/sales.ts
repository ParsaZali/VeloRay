export type MenuButton = {
  action: string;
  label: string;
  visible: boolean;
  row: number;
};
export type BotContent = {
  label: string;
  url: string;
  text: string;
  image: string;
};
export type Tutorial = {
  title: string;
  url: string;
  text: string;
  image: string;
};
export type SalesBot = {
  locale: string;
  menu_mode: string;
  welcome: string;
  welcome_image: string;
  catalog_text: string;
  catalog_image: string;
  empty_text: string;
  delivery_text: string;
  trial_text: string;
  support_text: string;
  tutorial_text: string;
  tutorial_image: string;
  buttons: MenuButton[];
  custom_buttons: BotContent[];
  tutorials: Tutorial[];
  trial_enabled: boolean;
  trial_mode: string;
  trial_service_id: number;
  trial_traffic_bytes: number;
  trial_hours: number;
  trial_channel: string;
  trial_channel_url: string;
  trial_paid_only: boolean;
  allow_toggle: boolean;
  allow_rotate: boolean;
  card_enabled: boolean;
  card_number: string;
  card_holder: string;
  card_instructions: string;
  usdt_enabled: boolean;
  usdt_network: string;
  usdt_address: string;
  usdt_instructions: string;
};
export const defaultSalesBot: SalesBot = {
  locale: "fa",
  menu_mode: "inline",
  welcome: "",
  welcome_image: "",
  catalog_text: "",
  catalog_image: "",
  empty_text: "",
  delivery_text: "",
  trial_text: "",
  support_text: "",
  tutorial_text: "",
  tutorial_image: "",
  buttons: [
    { action: "buy", label: "🛍 خرید اشتراک", visible: true, row: 0 },
    { action: "services", label: "📦 سرویس‌های من", visible: true, row: 0 },
    { action: "trial", label: "🎁 تست رایگان", visible: true, row: 1 },
    { action: "account", label: "👤 حساب من", visible: true, row: 1 },
    { action: "tutorial", label: "📚 آموزش اتصال", visible: true, row: 2 },
    { action: "support", label: "💬 پشتیبانی", visible: true, row: 2 },
    { action: "miniapp", label: "🌐 فروشگاه", visible: true, row: 3 },
  ],
  custom_buttons: [],
  tutorials: [],
  trial_enabled: false,
  trial_mode: "once",
  trial_service_id: 0,
  trial_traffic_bytes: 1073741824,
  trial_hours: 24,
  trial_channel: "",
  trial_channel_url: "",
  trial_paid_only: false,
  allow_toggle: false,
  allow_rotate: false,
  card_enabled: false,
  card_number: "",
  card_holder: "",
  card_instructions: "",
  usdt_enabled: false,
  usdt_network: "TRC20",
  usdt_address: "",
  usdt_instructions: "",
};
export type ManualMethod = {
  method: "card" | "usdt";
  card_number?: string;
  card_holder?: string;
  network?: string;
  address?: string;
  instructions: string;
};
export type PaymentProof = {
  id: number;
  order_id: string;
  method: string;
  state: string;
  network: string;
  transaction_id?: string;
  created_at: string;
  review_note: string;
  evidence?: { image?: string; note?: string; destination?: ManualMethod };
};
export type BuyerService = {
  id: number;
  name: string;
  enabled: boolean;
  used_traffic_bytes: number;
  traffic_limit_bytes: number;
  remaining_bytes: number | null;
  unlimited: boolean;
  expires_at?: string;
  subscription_url: string;
};
export type Ticket = {
  id: number;
  user_id: number;
  subject: string;
  state: string;
  updated_at: string;
};
export type TicketMessage = {
  id: number;
  author: string;
  text: string;
  created_at: string;
};
