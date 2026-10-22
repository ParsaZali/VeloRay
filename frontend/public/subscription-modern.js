(() => {
"use strict";
const root=document.querySelector(".portal-shell");
if(!root)return;
const byId=id=>document.getElementById(id);
const messages={
en:{noExpiry:"No expiry",clientApp:"Application",autoApp:"Auto detect / other",subscriptionCenter:"Subscription center",language:"Language",theme:"Toggle theme",refresh:"Refresh",yourSubscription:"YOUR SUBSCRIPTION",manageConnection:"Manage your subscription and connection profiles in one place.",active:"Active",usedTraffic:"Used traffic",outOf:"out of",remaining:"Remaining",trafficRemaining:"Available traffic",expiry:"Expiry date",expiryDescription:"Subscription expiration",connections:"Connections",availableProfiles:"Available connection profiles",analytics:"ANALYTICS",trafficOverview:"Traffic overview",totalTraffic:"Total",uploadTraffic:"Upload",downloadTraffic:"Download",loadingTraffic:"Loading real usage history…",noHistory:"No recorded usage for this period.",actualUsage:"Actual usage, not estimated",downloadCsv:"Download CSV",quota:"QUOTA",dataAllowance:"Data allowance",consumed:"consumed",rawTraffic:"Raw traffic",billingRate:"Billing multiplier",quotaNote:"Usage is counted from server accounting.",registeredDevices:"Registered devices",lastActivity:"Last usage",subscriptionLink:"SUBSCRIPTION LINK",yourLink:"Your universal link",importHelp:"Copy the link and import it into a compatible client to receive configuration updates automatically.",url:"Subscription URL",copyLink:"Copy link",qr:"QR code",jsonDetails:"JSON details",profiles:"PROFILES",connectionProfiles:"Connection profiles",copyAll:"Copy all",search:"Search profiles",searchHint:"Search name, protocol or server",allProtocols:"All protocols",noProfiles:"No profiles match your search.",compatibleApps:"CLIENT SOFTWARE",recommendedApps:"Recommended applications",appsHelp:"Choose a trusted application for your device, then import the subscription URL.",powered:"Subscription portal",contactSupport:"Contact support",qrHelp:"Scan with a compatible VPN application.",close:"Close",copyProfile:"Copy profile",downloadProfile:"Download profile",qrProfile:"Profile QR code",more:"More ↗",copied:"Link copied",copyFailed:"Please select and copy the link",refreshFailed:"Traffic history could not load.",unlimited:"Unlimited traffic",remainingDays:"days remaining",expired:"Expired",expiresToday:"Expires today",alreadyExpired:"Subscription expired",today:"today"},
fa:{noExpiry:"بدون انقضا",clientApp:"برنامه اتصال",autoApp:"تشخیص خودکار / سایر",subscriptionCenter:"مرکز اشتراک",language:"زبان",theme:"تغییر تم",refresh:"تازه‌سازی",yourSubscription:"اشتراک شما",manageConnection:"وضعیت اشتراک و کانفیگ‌های اتصال را در یک صفحه مدیریت کنید.",active:"فعال",usedTraffic:"مصرف ترافیک",outOf:"از",remaining:"باقی‌مانده",trafficRemaining:"ترافیک قابل استفاده",expiry:"تاریخ انقضا",expiryDescription:"وضعیت اعتبار اشتراک",connections:"اتصالات",availableProfiles:"کانفیگ‌های در دسترس",analytics:"گزارش مصرف",trafficOverview:"نمودار مصرف ترافیک",totalTraffic:"کل مصرف",uploadTraffic:"آپلود",downloadTraffic:"دانلود",loadingTraffic:"در حال دریافت تاریخچه واقعی مصرف…",noHistory:"برای این بازه مصرفی ثبت نشده است.",actualUsage:"مصرف واقعی، بدون برآورد",downloadCsv:"دریافت CSV",quota:"سهمیه",dataAllowance:"سهمیه ترافیک",consumed:"مصرف‌شده",rawTraffic:"ترافیک واقعی",billingRate:"ضریب محاسبه",quotaNote:"مصرف براساس آمار ثبت‌شده سرور محاسبه می‌شود.",registeredDevices:"دستگاه‌های ثبت‌شده",lastActivity:"آخرین مصرف",subscriptionLink:"لینک اشتراک",yourLink:"لینک اصلی شما",importHelp:"لینک را کپی و در برنامه سازگار وارد کنید تا کانفیگ‌ها خودکار به‌روزرسانی شوند.",url:"آدرس اشتراک",copyLink:"کپی لینک",qr:"کد QR",jsonDetails:"جزئیات JSON",profiles:"کانفیگ‌ها",connectionProfiles:"کانفیگ‌های اتصال",copyAll:"کپی همه",search:"جستجوی کانفیگ",searchHint:"جستجوی نام، پروتکل یا سرور",allProtocols:"همه پروتکل‌ها",noProfiles:"کانفیگی مطابق جستجو وجود ندارد.",compatibleApps:"برنامه‌های اتصال",recommendedApps:"برنامه‌های پیشنهادی",appsHelp:"برنامه معتبر مخصوص دستگاه خود را انتخاب کنید و لینک اشتراک را وارد کنید.",powered:"صفحه اشتراک",contactSupport:"ارتباط با پشتیبانی",qrHelp:"با یک برنامه VPN سازگار اسکن کنید.",close:"بستن",copyProfile:"کپی کانفیگ",downloadProfile:"دانلود کانفیگ",qrProfile:"کد QR کانفیگ",more:"بیشتر ↗",copied:"لینک کپی شد",copyFailed:"لینک را انتخاب و کپی کنید",refreshFailed:"دریافت تاریخچه مصرف ناموفق بود.",unlimited:"ترافیک نامحدود",remainingDays:"روز تا انقضا",expired:"منقضی‌شده",expiresToday:"انقضا امروز",alreadyExpired:"اشتراک منقضی شده",today:"امروز"},
ru:{noExpiry:"Без срока действия",clientApp:"Приложение",autoApp:"Автоматически / другое",subscriptionCenter:"Подписка",language:"Язык",theme:"Тема",refresh:"Обновить",yourSubscription:"ВАША ПОДПИСКА",manageConnection:"Управляйте подпиской и подключениями.",active:"Активна",usedTraffic:"Использовано",outOf:"из",remaining:"Осталось",trafficRemaining:"Доступный трафик",expiry:"Окончание",expiryDescription:"Срок подписки",connections:"Подключения",availableProfiles:"Доступные профили",analytics:"СТАТИСТИКА",trafficOverview:"Использование трафика",totalTraffic:"Всего",uploadTraffic:"Отправлено",downloadTraffic:"Получено",loadingTraffic:"Загрузка истории трафика…",noHistory:"Нет данных за этот период.",actualUsage:"Реальные данные сервера",downloadCsv:"Скачать CSV",quota:"ЛИМИТ",dataAllowance:"Лимит трафика",consumed:"использовано",rawTraffic:"Реальный трафик",billingRate:"Множитель",quotaNote:"Данные получены с сервера.",registeredDevices:"Устройства",lastActivity:"Последняя активность",subscriptionLink:"ССЫЛКА",yourLink:"Ссылка подписки",importHelp:"Скопируйте ссылку и импортируйте её в клиент.",url:"URL подписки",copyLink:"Копировать",qr:"QR-код",jsonDetails:"JSON",profiles:"ПРОФИЛИ",connectionProfiles:"Профили подключения",copyAll:"Копировать все",search:"Поиск",searchHint:"Имя, протокол или сервер",allProtocols:"Все протоколы",noProfiles:"Профили не найдены.",compatibleApps:"ПРИЛОЖЕНИЯ",recommendedApps:"Рекомендуемые приложения",appsHelp:"Выберите приложение для устройства.",powered:"Страница подписки",contactSupport:"Поддержка",qrHelp:"Отсканируйте код в приложении.",close:"Закрыть",copyProfile:"Копировать профиль",downloadProfile:"Скачать профиль",qrProfile:"QR-код профиля",more:"Подробнее ↗",copied:"Ссылка скопирована",copyFailed:"Выберите и скопируйте ссылку",refreshFailed:"Ошибка загрузки истории.",unlimited:"Без ограничений",remainingDays:"дней осталось",expired:"Истекла",expiresToday:"Истекает сегодня",alreadyExpired:"Подписка истекла",today:"сегодня"},
zh:{noExpiry:"永不过期",clientApp:"客户端",autoApp:"自动 / 其他",subscriptionCenter:"订阅中心",language:"语言",theme:"切换主题",refresh:"刷新",yourSubscription:"您的订阅",manageConnection:"在一个页面管理订阅和连接配置。",active:"有效",usedTraffic:"已用流量",outOf:"总计",remaining:"剩余",trafficRemaining:"可用流量",expiry:"到期日期",expiryDescription:"订阅有效期",connections:"连接",availableProfiles:"可用配置",analytics:"统计",trafficOverview:"流量概览",totalTraffic:"总计",uploadTraffic:"上传",downloadTraffic:"下载",loadingTraffic:"正在加载真实流量…",noHistory:"该时间段没有记录。",actualUsage:"服务器实际用量",downloadCsv:"下载 CSV",quota:"配额",dataAllowance:"流量配额",consumed:"已使用",rawTraffic:"原始流量",billingRate:"计费倍率",quotaNote:"使用服务器统计数据。",registeredDevices:"注册设备",lastActivity:"最后使用",subscriptionLink:"订阅链接",yourLink:"通用链接",importHelp:"复制链接并导入兼容的客户端，配置会自动更新。",url:"订阅地址",copyLink:"复制链接",qr:"二维码",jsonDetails:"JSON 详情",profiles:"配置",connectionProfiles:"连接配置",copyAll:"全部复制",search:"搜索配置",searchHint:"搜索名称、协议或服务器",allProtocols:"全部协议",noProfiles:"没有匹配的配置。",compatibleApps:"客户端",recommendedApps:"推荐应用",appsHelp:"选择适合您设备的可信应用。",powered:"订阅页面",contactSupport:"联系支持",qrHelp:"使用兼容 VPN 客户端扫描。",close:"关闭",copyProfile:"复制配置",downloadProfile:"下载配置",qrProfile:"配置二维码",more:"更多 ↗",copied:"链接已复制",copyFailed:"请选择并复制链接",refreshFailed:"无法加载流量历史。",unlimited:"无限流量",remainingDays:"天后到期",expired:"已过期",expiresToday:"今天到期",alreadyExpired:"订阅已过期",today:"今天"}
};
function storageGet(k){try{return localStorage.getItem(k)}catch{return null}}
function storageSet(k,v){try{localStorage.setItem(k,v)}catch{}}
const allowedLanguages=["fa","en","ru","zh"];
let lang=storageGet("veloray-subscription-lang");
if(!allowedLanguages.includes(lang))lang=document.documentElement.lang;
if(!allowedLanguages.includes(lang))lang="en";
const dict=()=>messages[lang]||messages.en;
function translate(){
 document.documentElement.lang=lang;
 document.documentElement.dir=lang==="fa"?"rtl":"ltr";
 const m=dict();
 document.querySelectorAll("[data-i18n]").forEach(el=>{const key=el.getAttribute("data-i18n");if(key in m)el.textContent=m[key]});
 document.querySelectorAll("[data-i18n-aria]").forEach(el=>{const key=el.getAttribute("data-i18n-aria");if(key in m)el.setAttribute("aria-label",m[key])});
 document.querySelectorAll("[data-i18n-placeholder]").forEach(el=>{const key=el.getAttribute("data-i18n-placeholder");if(key in m)el.setAttribute("placeholder",m[key])});
 const copy=byId("copy-subscription");if(copy){copy.dataset.success=m.copied;copy.dataset.error=m.copyFailed}
 const activity=byId("last-activity");
 if(activity){const d=Date.parse(activity.getAttribute("datetime")||"");if(Number.isFinite(d)){const locale=lang==="fa"?"fa-IR-u-ca-persian":lang==="zh"?"zh-CN":lang==="ru"?"ru-RU":"en-US";activity.textContent=new Intl.DateTimeFormat(locale,{year:"numeric",month:"short",day:"numeric",hour:"2-digit",minute:"2-digit",timeZone:"UTC"}).format(d)}}
 updateCountdown();
}
byId("portal-language").value=lang;
byId("portal-language").addEventListener("change",ev=>{const next=ev.target.value;if(allowedLanguages.includes(next)){lang=next;storageSet("veloray-subscription-lang",lang);translate()}});
function preferredTheme(){const manual=storageGet("veloray-subscription-theme");if(manual==="light"||manual==="dark")return manual;const configured=document.documentElement.dataset.defaultTheme;if(configured==="dark"||configured==="light")return configured;return matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"}
function applyTheme(){document.documentElement.dataset.theme=preferredTheme()}
applyTheme();
byId("toggle-theme").addEventListener("click",()=>{storageSet("veloray-subscription-theme",preferredTheme()==="dark"?"light":"dark");applyTheme()});
if(window.matchMedia){const mq=matchMedia("(prefers-color-scheme: dark)");mq.addEventListener?.("change",()=>{if(!storageGet("veloray-subscription-theme"))applyTheme()})}
byId("refresh-portal").addEventListener("click",()=>location.reload());
const clock=root.getAttribute("data-client-expiry");
function updateCountdown(){
 const el=byId("expiry-countdown");if(!el)return;
 const dateLabel=document.querySelector(".stat-date");
 const parsed=clock?Date.parse(clock):NaN;
 if(dateLabel){
  if(Number.isFinite(parsed)){
   const locale=lang==="fa"?"fa-IR-u-ca-persian":lang==="zh"?"zh-CN":lang==="ru"?"ru-RU":"en-US";
   try{dateLabel.textContent=new Intl.DateTimeFormat(locale,{year:"numeric",month:"short",day:"numeric",timeZone:"UTC"}).format(new Date(parsed))}
   catch{dateLabel.textContent=new Date(parsed).toISOString().slice(0,10)}
  }else{dateLabel.textContent=dict().noExpiry}
 }
 if(!clock){el.textContent=dict().noExpiry;return}
 const end=Date.parse(clock);if(!Number.isFinite(end)){el.textContent=dict().expiryDescription;return}
 const diff=end-Date.now();if(diff<0){el.textContent=dict().alreadyExpired;return}
 const days=Math.ceil(diff/86400000);el.textContent=days<=1?dict().expiresToday:(new Intl.NumberFormat(lang).format(days)+" "+dict().remainingDays)
}
translate();
setInterval(updateCountdown,60000);
async function copyText(value,input){
 try{if(navigator.clipboard&&navigator.clipboard.writeText){await navigator.clipboard.writeText(value);return true}}catch{}
 if(!input)return false;
 input.focus();input.select();
 try{return document.execCommand("copy")}catch{return false}
}
const copyFeedback=byId("copy-feedback");
function copiedFeedback(button,ok){
 const text=ok?dict().copied:dict().copyFailed;
 if(copyFeedback)copyFeedback.textContent=text;
 if(button){
  const before=button.textContent;
  button.textContent=ok?"✓":dict().copyLink;
  setTimeout(()=>button.textContent=before,1500);
 }
}
byId("copy-subscription")?.addEventListener("click",async e=>copiedFeedback(e.currentTarget,await copyText(byId("subscription").value,byId("subscription"))));
document.querySelectorAll(".copy-profile").forEach(btn=>btn.addEventListener("click",async()=>{
 const input=btn.closest(".profile-item")?.querySelector(".profile-uri");
 if(input)copiedFeedback(btn,await copyText(input.value,input));
}));
document.querySelectorAll(".download-profile").forEach(button=>button.addEventListener("click",()=>{
 const input=button.closest(".profile-item")?.querySelector(".profile-uri");
 if(!input||!input.value)return;
 const content=new Blob([input.value+"\n"],{type:"text/plain;charset=utf-8"});
 const link=document.createElement("a");
 const name=button.closest(".profile-item")?.querySelector(".profile-description strong")?.textContent||"connection";
 link.download=(name.replace(/[^a-z0-9_-]/gi,"_").slice(0,48)||"connection")+(input.value.startsWith("[Interface]\n")?".conf":".txt");
 link.href=URL.createObjectURL(content);
 document.body.appendChild(link);
 link.click();
 link.remove();
 window.setTimeout(()=>URL.revokeObjectURL(link.href),1000);
}));
byId("copy-all")?.addEventListener("click",async e=>{
 const el=byId("all-configs");
 if(el)copiedFeedback(e.currentTarget,await copyText(el.value,el));
});
const modal=byId("qr-dialog");
document.querySelectorAll(".qr-trigger").forEach(btn=>btn.addEventListener("click",()=>{
 const src=btn.dataset.qr||"";
 const qrURL=new URL(src,location.href);
 if(!src.startsWith("data:image/png;base64,") && !(qrURL.origin===location.origin && qrURL.pathname.startsWith("/sub/") && qrURL.searchParams.get("format")==="qr"))return;
 byId("qr-preview").src=src;
 byId("qr-label").textContent=btn.dataset.label||"";
 if(typeof modal.showModal==="function")modal.showModal();
}));
byId("close-qr")?.addEventListener("click",()=>modal.close());
modal?.addEventListener("click",e=>{if(e.target===modal)modal.close()});
const search=byId("profile-search"),filter=byId("protocol-filter");
function filterProfiles(){
 const q=(search?.value||"").toLocaleLowerCase(),protocol=filter?.value||"all";
 let shown=0;
 document.querySelectorAll(".profile-item").forEach(el=>{
  const match=(!q||(el.dataset.search||"").toLocaleLowerCase().includes(q))&&(protocol==="all"||el.dataset.protocol===protocol);
  el.hidden=!match;if(match)shown++;
 });
 if(byId("profiles-empty"))byId("profiles-empty").hidden=shown>0
}
search?.addEventListener("input",filterProfiles);filter?.addEventListener("change",filterProfiles);
function detectOS(){
 const ua=navigator.userAgent.toLowerCase();
 if(/android/.test(ua))return "android";
 if(/iphone|ipad|ipod/.test(ua))return "ios";
 if(/windows/.test(ua))return "windows";
 if(/macintosh|mac os/.test(ua))return "macos";
 if(/linux/.test(ua))return "linux";
 return "other";
}
const os=detectOS();
if(byId("detected-os"))byId("detected-os").textContent=os==="other"?"":os.toUpperCase();
const apps=[...document.querySelectorAll(".app-grid a")];
apps.sort((a,b)=>Number((b.dataset.platforms||"").split(" ").includes(os))-Number((a.dataset.platforms||"").split(" ").includes(os)));
const container=document.querySelector(".app-grid");if(container)apps.forEach(a=>container.appendChild(a));
})();
