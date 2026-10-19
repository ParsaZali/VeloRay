const entries = `
Install app|نصب برنامه
Update app|به‌روزرسانی برنامه

Recent subscription requests|درخواست‌های اخیر اشتراک
Rotate subscription|تعویض لینک اشتراک
All existing subscription URLs will stop working. Share the new URL with this customer.|همه لینک‌های اشتراک قبلی از کار می‌افتند. لینک جدید را به این مشتری بدهید.

Customers|مشتریان
Services and plans|سرویس‌ها و پلن‌ها
Config profiles|پروفایل‌های تنظیمات
Add customer|افزودن مشتری
Add service|افزودن سرویس
Add profile|افزودن پروفایل
Add snippet|افزودن اسنیپت
Service|سرویس
Customer|مشتری
Profile|پروفایل
Snippet|اسنیپت
Profiles|پروفایل‌ها
Snippets|اسنیپت‌ها
Revision|نسخه
Assign nodes|انتخاب نودها
No records yet|هنوز موردی ثبت نشده است
No inbounds available|اینباندی در دسترس نیست
Quota and access|سهمیه و دسترسی
Traffic quota (GiB)|سهمیه ترافیک (گیبی‌بایت)
Concurrent IP limit|محدودیت آی‌پی هم‌زمان
Renewal interval (days)|فاصله تمدید (روز)
Duration after first use (days)|مدت پس از اولین استفاده (روز)
Start duration after first use|شروع مدت از اولین استفاده
Apply plan defaults|اعمال مقادیر پلن
Expiry (UTC)|انقضا (UTC)
Calendar renewal|تمدید تقویمی
Use day interval|استفاده از فاصله روزانه
Hourly|ساعتی
Daily|روزانه
Weekly|هفتگی
Monthly|ماهانه
Every|هر
Timezone|منطقه زمانی
Xray policy JSON|سیاست Xray به صورت JSON
sing-box policy JSON|سیاست sing-box به صورت JSON
Policy JSON|سیاست به صورت JSON
Services grant access to their inbounds. Plan defaults apply when selected for a customer.|سرویس دسترسی به ایـنباندهای انتخاب‌شده را می‌دهد. مقادیر پلن هنگام انتخاب برای مشتری اعمال می‌شوند.
One account shares quota, expiry and subscription across inbounds and nodes.|یک حساب، سهمیه و انقضا و اشتراک مشترک روی چند ایـنباند و نود دارد.
Share network policy across nodes. Changes deploy together. The last 50 revisions can be restored.|سیاست شبکه را میان نودها مشترک کنید. تغییرات با هم اعمال می‌شوند و ۵۰ نسخه آخر قابل بازگردانی هستند.
Allowed sections: DNS, routing, outbounds and logging; Xray also supports policy. Managed listeners and telemetry remain controlled by the panel.|بخش‌های مجاز: DNS، مسیریابی، خروجی‌ها و لاگ؛ در Xray بخش policy هم مجاز است. ورودی‌ها و آمار تحت مدیریت پنل باقی می‌مانند.
HWID: 0 inherits the global limit; -1 disables it. Zero quota or IP limit means unlimited.|در HWID، صفر یعنی محدودیت سراسری و منفی یک یعنی غیرفعال. سهمیه یا محدودیت آی‌پی صفر یعنی نامحدود.

Protocol detection|تشخیص پروتکل

Outbound tags|تگ‌های خروجی
sing-box blocking rules must use the reject action|برای مسدودسازی در sing-box از reject استفاده کنید

Upload|آپلود
Download|دانلود
Earlier traffic|ترافیک قدیمی
24h usage|مصرف ۲۴ ساعت
Built-in|پیش‌فرض
Default route|مسیر پیش‌فرض
Panel API|رابط پنل
Node rule|قانون نود
New rule|قانون جدید
New outbound|خروجی جدید
Rules|قوانین
Configured outbounds|خروجی‌های تنظیم‌شده
No custom rules|قانون سفارشی وجود ندارد
No custom outbounds|خروجی سفارشی وجود ندارد
Name or match|نام یا شرط
Name or tag|نام یا تگ
Duplicate|کپی
Server address|آدرس سرور
Server port|پورت سرور
Domains|دامنه‌ها
Destination IPs|آی‌پی‌های مقصد
Source IPs|آی‌پی‌های مبدا
Inbound tags|تگ‌های ورودی
Protocols|پروتکل‌ها
Destination|مقصد
Network|شبکه
Any|همه
Block|مسدود
All traffic|تمام ترافیک
Block private IPs|مسدودسازی آی‌پی خصوصی
Block BitTorrent|مسدودسازی بیت‌تورنت
Direct HTTPS|ارتباط مستقیم HTTPS
Block ads|مسدودسازی تبلیغات
Advanced configuration|تنظیمات پیشرفته
Invalid port or range|پورت یا بازه نامعتبر است

0 means no expiry.|۰ یعنی بدون انقضا.
0 means unlimited.|۰ یعنی نامحدود.
1 day|۱ روز
1. Create an inbound|۱. ایجاد اینباند
2. Add a client|۲. افزودن کلاینت
24h ago|۲۴ ساعت قبل
2FA|احراز هویت دومرحله‌ای
2FA enabled|احراز هویت دومرحله‌ای فعال
2FA policy needs attention|سیاست احراز هویت نیاز به بررسی دارد
3. Share the subscription|۳. اشتراک‌گذاری لینک
6-digit code|کد شش‌رقمی
API|رابط برنامه‌نویسی
API key copied.|کلید دسترسی کپی شد.
API key created. Copy it now; the full token is shown only once.|کلید دسترسی ساخته شد. اکنون کپی کنید؛ فقط یک بار نمایش داده می‌شود.
API keys|کلیدهای دسترسی
API reference|راهنمای رابط برنامه‌نویسی
Accent|رنگ تأکیدی
Account, client and server changes.|تغییرات حساب‌ها، کلاینت‌ها و سرورها.
Accounts|حساب‌ها
Across all clients|مجموع کلاینت‌ها
Action|عملیات
Actions|عملیات
Active|فعال
Active API keys|کلیدهای دسترسی فعال
Active account|حساب فعال
Active clients|کلاینت‌های فعال
Actor|انجام‌دهنده
Actual:|واقعی:
Add DNS, routing and outbound settings. Xray validates the result before applying it.|تنظیمات دی‌ان‌اس، مسیریابی و خروجی را اضافه کنید. هسته پیش از اجرا آن را اعتبارسنجی می‌کند.
Add account|افزودن حساب
Edit account|ویرایش حساب
Save account|ذخیره حساب
Account saved.|حساب ذخیره شد.
Reseller|فروشنده
Viewer|مشاهده‌گر
Custom access|دسترسی سفارشی
Own clients only|فقط کلاینت‌های خود حساب
Client limit|سقف تعداد کلاینت
Traffic budget (GB)|بودجه ترافیک (گیگابایت)
Assigned inbounds|اینباندهای اختصاص‌یافته
Assign an inbound before this reseller can create clients.|برای ساخت کلاینت توسط فروشنده، یک اینباند اختصاص دهید.
Permissions|دسترسی‌ها
View dashboard|مشاهده داشبورد
View clients|مشاهده کلاینت‌ها
Create and edit clients|ایجاد و ویرایش کلاینت‌ها
Delete clients|حذف کلاینت‌ها
View inbounds|مشاهده اینباندها
Manage inbounds|مدیریت اینباندها
View nodes|مشاهده نودها
Manage nodes|مدیریت نودها
View traffic|مشاهده ترافیک
View Xray|مشاهده هسته
Manage Xray|مدیریت هسته
View settings|مشاهده تنظیمات
View API reference|مشاهده راهنمای رابط
Access is not assigned to this account.|این دسترسی به حساب شما اختصاص نیافته است.
Administrators, resellers and custom access.|مدیران، فروشندگان و دسترسی سفارشی.
Add administrator|افزودن مدیر
Add client|افزودن کلاینت
Add remote node|افزودن نود خارجی
Add the secret to your authenticator, then confirm a code.|کلید را به برنامه احراز هویت اضافه و سپس یک کد تأیید کنید.
Administration|حساب‌ها
Administrator|مدیر
Administrator 2FA and account protection|احراز هویت دومرحله‌ای و حفاظت حساب
Administrator authentication and automation policy.|سیاست احراز هویت مدیران و خودکارسازی.
Administrator saved.|حساب مدیر ذخیره شد.
Administrators|مدیران
Admins|مدیران
Advanced merge patch|تنظیمات تکمیلی ادغام
Advanced patch must be a JSON object.|تنظیمات تکمیلی باید یک شیء جی‌سان باشد.
Advanced patch must be valid JSON.|تنظیمات تکمیلی باید جی‌سان معتبر باشد.
Advanced patch saved, validated and deployed.|تنظیمات تکمیلی ذخیره، اعتبارسنجی و اجرا شد.
Advanced stream settings|تنظیمات پیشرفته انتقال
Agent URL|نشانی عامل
Agent auth|احراز هویت عامل
Agent bearer tokens are encrypted at rest.|کلیدهای عامل به صورت رمزگذاری‌شده ذخیره می‌شوند.
Agent token|کلید عامل
Agent transport|ارتباط عامل
Alert cooldown (minutes)|فاصله هشدارها (دقیقه)
All inbounds|همه اینباندها
All status|همه وضعیت‌ها
Allow bot management|اجازه مدیریت با ربات
Allowed network|شبکه مجاز
Announcement|اعلان
Announcement URL|نشانی اعلان
Apply an action to 1–200 clients atomically|اجرای یک عملیات مشترک روی ۱ تا ۲۰۰ کلاینت
Audit log|گزارش رویدادها
Audit retention|نگهداری گزارش رویدادها
Authenticator / recovery code|کد احراز هویت یا بازیابی
Authenticator secret|کلید احراز هویت
Automatic route CIDRs|محدوده‌های مسیریابی خودکار
Automation|خودکارسازی
Available|در دسترس
Backup, restore and admin recovery remain available through the host CLI.|پشتیبان‌گیری، بازیابی و بازیابی مدیر از خط فرمان سرور در دسترس است.
Base64 Xray feed|خروجی کدگذاری‌شده هسته
Batch create|ساخت گروهی
Batch create clients|ساخت گروهی کلاینت‌ها
Billed traffic|مصرف محاسبه‌شده
Bot check failed|بررسی ربات ناموفق بود
Bot connection checked.|ارتباط ربات بررسی شد.
Bot identity and polling status|مشخصات ربات و وضعیت دریافت پیام
Bot language|زبان ربات
Bot token|کلید ربات
Browser API uses secure sessions and CSRF protection. Public subscription routes are token-scoped and do not expose panel administration.|مرورگر از نشست امن و محافظت در برابر جعل درخواست استفاده می‌کند. لینک‌های اشتراک دسترسی محدود به کلید دارند.
Browser portal or auto-detected client feed|صفحه مرورگر یا خروجی خودکار کلاینت
CPU|پردازنده
CPU alert %|درصد هشدار پردازنده
Cancel|لغو
Change the filters or create your first client.|فیلترها را تغییر دهید یا اولین کلاینت خود را بسازید.
Changes are validated and deployed to Xray automatically.|تغییرات به‌صورت خودکار اعتبارسنجی و روی هسته اجرا می‌شوند.
Check a proposed inbound port|بررسی پورت پیشنهادی اینباند
Check bot connection|بررسی ارتباط ربات
Check listeners and current socket owners|بررسی پورت‌های باز و سرویس استفاده‌کننده
Check port|بررسی پورت
Checking listeners…|در حال بررسی پورت‌ها…
Checking port…|در حال بررسی پورت…
Choose a port, protocol and connection security.|پورت، پروتکل و امنیت اتصال را انتخاب کنید.
Choose a starting point|انتخاب نقطه شروع
Cipher|روش رمزگذاری
Clash/Mihomo YAML|خروجی یامل کلش و میهومو
Clean subscription URL|نشانی اصلی اشتراک
Client|کلاینت
Client DNS|دی‌ان‌اس کلاینت
Client created and Xray redeployed.|کلاینت ساخته و هسته به‌روز شد.
Client defaults|پیش‌فرض کلاینت‌ها
Client directory|فهرست کلاینت‌ها
Client health|وضعیت کلاینت‌ها
Client name|نام کلاینت
Client traffic and server status.|ترافیک کلاینت‌ها و وضعیت سرور.
Client updated and Xray redeployed.|کلاینت و هسته به‌روز شدند.
Client usage and traffic history|مصرف کلاینت‌ها و تاریخچه ترافیک
Clients|کلاینت‌ها
Clients ranked by traffic|کلاینت‌ها به ترتیب مصرف
Clipboard is unavailable. Download the configuration instead.|کپی در دسترس نیست. پیکربندی را دانلود کنید.
Clone|ساخت نسخه
Clone & deploy|ساخت نسخه و اجرا
Clone an inbound to a node|ساخت نسخه اینباند روی یک نود
Clone inbound|ساخت نسخه اینباند
Close|بستن
Close menu|بستن منو
Collapse navigation|جمع‌کردن منو
Commands:|دستورها:
Compact portal|صفحه فشرده
Configuration copied.|پیکربندی کپی شد.
Configuration deployed.|پیکربندی اجرا شد.
Configuration for the selected node. Downloads contain private connection credentials.|پیکربندی نود انتخاب‌شده. فایل دانلودی شامل اطلاعات خصوصی اتصال است.
Configuration is valid. The running service was not changed.|پیکربندی معتبر است. سرویس فعلی تغییر نکرد.
Configure transport, ALPN, socket and TLS options. Ports, credentials and security mode use the fields above. Xray validates changes before applying them.|گزینه‌های انتقال، مذاکره پروتکل، سوکت و امنیت را تنظیم کنید. پورت و اطلاعات اتصال در قسمت بالا قرار دارند. هسته تغییرات را پیش از اجرا بررسی می‌کند.
Configured inbound families|خانواده‌های اینباند تنظیم‌شده
Configured inbounds|اینباندهای تنظیم‌شده
Confirm & enable|تأیید و فعال‌سازی
Conflict|تداخل
Connection URI|نشانی اتصال
Connection preset|پیش‌تنظیم اتصال
Connection protocols, ports and server credentials.|پروتکل‌ها، پورت‌ها و اطلاعات اتصال سرور.
Copied|کپی شد
Copy|کپی
Copy config|کپی پیکربندی
Copy link|کپی لینک
Copy sub|کپی اشتراک
Copy subscription|کپی اشتراک
Create & deploy|ایجاد و اجرا
Create 1–100 clients in one deployment|ایجاد ۱ تا ۱۰۰ کلاینت در یک عملیات
Create API key|ساخت کلید دسترسی
Create a client to generate a subscription endpoint.|برای ساخت لینک اشتراک یک کلاینت بسازید.
Create client|ساخت کلاینت
Create clients|ساخت کلاینت‌ها
Create key|ساخت کلید
Create your first listener on Local Node.|اولین اینباند را روی نود محلی بسازید.
Credentials in portal|اطلاعات اتصال در صفحه اشتراک
Credentials, quotas, expiry and subscription access|اطلاعات اتصال، سهمیه، انقضا و اشتراک
Current availability|وضعیت دسترسی فعلی
Current password|رمز عبور فعلی
Current socket owners for this configuration. Refresh after changing a port.|سرویس‌های فعلی استفاده‌کننده از پورت‌ها. پس از تغییر پورت به‌روز کنید.
DNS servers|سرورهای دی‌ان‌اس
Dashboard|داشبورد
Dashboard refresh|فاصله به‌روزرسانی داشبورد
Dashboard totals|آمار داشبورد
Database|پایگاه داده
Default API key lifetime|عمر پیش‌فرض کلید دسترسی
Default expiry (days)|انقضای پیش‌فرض (روز)
Default locale|زبان پیش‌فرض
Default renewal (days)|تمدید پیش‌فرض (روز)
Default traffic (GB)|ترافیک پیش‌فرض (گیگابایت)
Defaults used by Client creation.|پیش‌فرض‌های ساخت کلاینت.
Delete|حذف
Deploy|اجرا
Deploy:|اجرا:
Description|توضیح
Destination|مقصد
Detail|جزئیات
Diagnostics|عیب‌یابی
Direct connection URI|نشانی اتصال مستقیم
Disable|غیرفعال‌کردن
Disable 2FA|غیرفعال‌کردن احراز هویت دومرحله‌ای
Disabled|غیرفعال
Disabled in Settings|در تنظیمات غیرفعال است
Disabled without per-user metering.|بدون محاسبه مصرف کاربر غیرفعال است.
Disk|دیسک
Disk alert %|درصد هشدار دیسک
Download|دانلود
Duplicate|تکراری
Edit|ویرایش
Edit administrator|ویرایش مدیر
Edit client|ویرایش کلاینت
Edit inbound|ویرایش اینباند
Edit remote node|ویرایش نود خارجی
Email|ایمیل
Enable|فعال‌کردن
Enable API keys|فعال‌کردن کلیدهای دسترسی
Enable UDP relay|فعال‌کردن انتقال یودی‌پی
Enable or disable client|فعال یا غیرفعال‌کردن کلاینت
Enabled|فعال
Enabled ·|فعال ·
English|انگلیسی
Persian|فارسی
Enter a number.|یک عدد وارد کنید.
Enter a valid traffic multiplier.|یک ضریب معتبر وارد کنید.
Existing webhook blocks polling|وب‌هوک موجود مانع دریافت پیام است
Expand navigation|بازکردن منو
Expired|منقضی
Expires|انقضا
Expires at|زمان انقضا
Expires in days|انقضا به روز
Expiring soon|نزدیک انقضا
Expiring within 7 days|انقضا در ۷ روز آینده
Expiry|انقضا
Expiry after (days)|انقضا پس از (روز)
Expiry warning (days)|هشدار انقضا (روز)
Export usage|خروجی مصرف
Feed behavior and public portal presentation.|رفتار خروجی و نمایش صفحه عمومی اشتراک.
Fleet|نودها
Follow redirected destination|پیروی از مقصد هدایت‌شده
Frankfurt 01|فرانکفورت ۰۱
Full portal|صفحه کامل
GB|گیگابایت
Gateway CIDRs|محدوده‌های دروازه
General|عمومی
Generate|ساخت
Generate X25519 keys on the selected node.|ساخت کلیدهای اتصال روی نود انتخاب‌شده.
Generate the server keypair, then check the port.|جفت کلید سرور را بسازید و سپس پورت را بررسی کنید.
Generated by Xray on the selected node.|ساخته‌شده توسط هسته روی نود انتخاب‌شده.
Generated config preview|پیش‌نمایش پیکربندی
Go to overview|رفتن به داشبورد
HTTP Proxy|پروکسی اچ‌تی‌تی‌پی
HTTP and SOCKS are plaintext proxy protocols. Prefer local/LAN use or place them behind a protected transport.|این پروتکل‌های پروکسی رمزگذاری ندارند. در شبکه محلی یا پشت یک ارتباط امن استفاده کنید.
Healthy nodes|نودهای سالم
Host|میزبان
Host header|سرآیند میزبان
Host health|وضعیت میزبان
ID|شناسه
IP|آی‌پی
Import data|واردسازی داده‌ها
Imports and Telegram (administrators)|واردسازی و تلگرام (مدیران)
In use by Xray|در استفاده هسته
Inbound|اینباند
Inbound created, validated and deployed.|اینباند ساخته، اعتبارسنجی و اجرا شد.
Inbound updated, validated and deployed.|اینباند ویرایش، اعتبارسنجی و اجرا شد.
Inbounds|اینباندها
Inbounds & clients|اینباندها و کلاینت‌ها
Infrastructure|زیرساخت
Inspect Xray|بررسی هسته
Interface name|نام رابط
Key name|نام کلید
Last login|آخرین ورود
Last seen|آخرین مشاهده
Last successful poll:|آخرین دریافت موفق:
Last used|آخرین استفاده
Leave blank to keep current token|برای حفظ کلید فعلی خالی بگذارید
Leave the token blank to keep the current secret.|برای حفظ کلید فعلی این قسمت را خالی بگذارید.
Limited|محدود
List nodes|فهرست نودها
List or create clients|فهرست یا ساخت کلاینت‌ها
List or create inbounds|فهرست یا ساخت اینباندها
Listen address|نشانی شنود
Listener|شنونده
Listener checks|بررسی پورت‌ها
Live connections · every 5 seconds|اتصال‌های زنده · هر ۵ ثانیه
Live server diagnostics.|عیب‌یابی زنده سرور.
Load average|میانگین بار
Loading…|در حال بارگذاری…
Local|محلی
Local Agent stays loopback-only; remote Agents require TLS.|عامل محلی فقط روی نشانی داخلی در دسترس است؛ عامل خارجی به ارتباط امن نیاز دارد.
Local node enrolled|نود محلی ثبت شد
Local server and remote VeloRay agents|سرور محلی و عامل‌های خارجی
Main|اصلی
Maintenance mode|حالت تعمیرات
Manage credentials, quotas, expiry, renewals and subscriptions.|مدیریت اطلاعات اتصال، سهمیه، انقضا، تمدید و اشتراک‌ها.
Manage inbounds →|مدیریت اینباندها ←
Manage the local server and your remote nodes.|مدیریت سرور محلی و نودهای خارجی.
Management|مدیریت
Masquerade URL|نشانی پوششی
Memory|حافظه
Memory alert %|درصد هشدار حافظه
Message sent to compatible subscription clients|پیام ارسالی به کلاینت‌های سازگار
Method|روش
Multipart file + node; private preview valid for 30 minutes|فایل و نود مقصد؛ پیش‌نمایش خصوصی با اعتبار ۳۰ دقیقه
Name|نام
Name prefix|پیشوند نام
Names use prefix-001, prefix-002… Each account gets separate credentials and a subscription address.|نام‌ها با پیشوند و شماره ساخته می‌شوند. هر حساب اطلاعات اتصال و لینک اشتراک جدا دارد.
Native|اصلی
Never|هرگز
New API key|کلید دسترسی جدید
New inbound|اینباند جدید
New password (optional)|رمز عبور جدید (اختیاری)
No API keys created.|کلید دسترسی ساخته نشده است.
No administrators found.|مدیری یافت نشد.
No audit events found.|رویدادی یافت نشد.
No client traffic yet.|هنوز مصرفی ثبت نشده است.
No clients found|کلاینتی یافت نشد
No configured IP listeners.|پورت تنظیم‌شده‌ای یافت نشد.
No email|بدون ایمیل
No expiry|بدون انقضا
No inbounds yet.|هنوز اینباندی وجود ندارد.
No nodes registered|نودی ثبت نشده است
No nodes yet. Add a remote node or check the local installation.|نودی وجود ندارد. نود خارجی اضافه کنید یا نصب محلی را بررسی کنید.
No source IP|بدون آی‌پی مبدا
No subscriptions found|اشتراکی یافت نشد
No token|بدون کلید
No traffic recorded yet.|هنوز ترافیکی ثبت نشده است.
No traffic yet|هنوز ترافیکی وجود ندارد
Node|نود
Node credentials|اطلاعات اتصال نود
Node name|نام نود
Nodes|نودها
Nodes need attention|نودها نیاز به بررسی دارند
None|هیچ‌کدام
Not configured|تنظیم نشده
Not metered per user|مصرف کاربر محاسبه نمی‌شود
Not yet|هنوز ثبت نشده
Note|یادداشت
Now|اکنون
Number of clients|تعداد کلاینت‌ها
Off|خاموش
Offline|آفلاین
One-time secret|کلید یک‌بارنمایش
Online|آنلاین
Online now|اکنون آنلاین
Only combinations validated by VeloRay are deployable.|فقط ترکیب‌های تأییدشده قابل اجرا هستند.
Open navigation|بازکردن منو
Open portal|بازکردن صفحه اشتراک
Open subscription portal|بازکردن صفحه اشتراک
Open the client portal or copy a connection link.|صفحه کلاینت را باز کنید یا لینک اتصال را کپی کنید.
Operating system|سیستم‌عامل
Operators|حساب‌ها
Operators, access policy and scoped API credentials.|حساب‌ها، سیاست دسترسی و کلیدهای محدود رابط برنامه‌نویسی.
Optional footer|پاورقی اختیاری
Optional internal note|یادداشت داخلی اختیاری
Outbound interface|رابط خروجی
Overview|نمای کلی
Owner|مالک
Owner IDs|شناسه‌های مالک
Owner command menu registered.|منوی دستورهای مالک ثبت شد.
Owner-only alerts and management commands.|هشدارها و دستورهای مدیریت مخصوص مالک.
Panel and nodes|پنل و نودها
Panel defaults and server information|پیش‌فرض پنل و اطلاعات سرور
Panel identity and interface defaults.|مشخصات پنل و پیش‌فرض رابط کاربری.
Panel name|نام پنل
Panel node and remote agents|نود پنل و عامل‌های خارجی
Panel operators, roles and account security|حساب‌ها، نقش‌ها و امنیت حساب
Panel server|سرور پنل
Panel users|کاربران پنل
Password|رمز عبور
Password is required for a new administrator.|رمز عبور برای حساب جدید الزامی است.
Paste the credentials printed by the node installer.|اطلاعات اتصال نمایش‌داده‌شده توسط نصب‌کننده نود را وارد کنید.
Paste token|واردکردن کلید
Path|مسیر
Pause|توقف
Paused|متوقف
Per-client traffic is unavailable for this protocol. Use unlimited traffic and no quota renewal.|محاسبه مصرف کلاینت برای این پروتکل در دسترس نیست. از ترافیک نامحدود و بدون تمدید سهمیه استفاده کنید.
Per-client usage|مصرف هر کلاینت
Per-user metering unavailable for this protocol.|محاسبه مصرف کاربر برای این پروتکل در دسترس نیست.
Polling available|دریافت پیام در دسترس است
Port|پورت
Port (not used)|پورت (استفاده نمی‌شود)
Port check failed.|بررسی پورت ناموفق بود.
Port check passed. Availability is checked again before deployment.|بررسی پورت موفق بود. پیش از اجرا دوباره بررسی می‌شود.
Portal footer|پاورقی صفحه اشتراک
Ports, protocols and connection security|پورت‌ها، پروتکل‌ها و امنیت اتصال
Prefix|پیشوند
Preview generated Xray JSON|پیش‌نمایش پیکربندی هسته
Private key|کلید خصوصی
Probe|بررسی
Probe Agent and Xray|بررسی عامل و هسته
Profile metadata JSON|مشخصات پروفایل به صورت جی‌سان
Protect your account with two-factor authentication.|با احراز هویت دومرحله‌ای از حساب خود محافظت کنید.
Protocol|پروتکل
Protocol mix|ترکیب پروتکل‌ها
Public client portals, auto-detected feeds, Clash/Mihomo output and direct connection links.|صفحه عمومی کلاینت، خروجی خودکار، خروجی کلش و میهومو و لینک اتصال مستقیم.
Public host|میزبان عمومی
Public key|کلید عمومی
QR code|کد تصویری
Quota and expiry alerts|هشدار سهمیه و انقضا
Quota limited|دارای سهمیه محدود
Quota managed|دارای سهمیه
Quota reached|سهمیه تمام‌شده
Quota renewal (days)|تمدید سهمیه (روز)
Quota warning %|درصد هشدار سهمیه
REALITY identity|مشخصات ریالیتی
REALITY key pair generated.|جفت کلید ریالیتی ساخته شد.
REST endpoints and automation reference|مسیرهای رابط برنامه‌نویسی و راهنمای خودکارسازی
Raw|خام
Read + write|خواندن و نوشتن
Read only|فقط خواندن
Read-only operator view. Product settings can only be changed by a superuser.|نمای فقط خواندنی. تغییر تنظیمات فقط توسط مدیر اصلی مجاز است.
Recent events|رویدادهای اخیر
Recommended apps|برنامه‌های پیشنهادی
Recorded traffic|ترافیک ثبت‌شده
Recorded usage|مصرف ثبت‌شده
Recovery|بازیابی
Recovery codes|کدهای بازیابی
Refresh|به‌روزرسانی
Register commands for private owner chats|ثبت دستورها برای گفت‌وگوی خصوصی مالک
Register owner commands|ثبت دستورهای مالک
Reload panel|بارگذاری مجدد پنل
Reload the panel to try again.|پنل را دوباره بارگذاری کنید.
Remote node|نود خارجی
Remote node saved. Probe it before deploying traffic.|نود خارجی ذخیره شد. پیش از اجرای ترافیک آن را بررسی کنید.
Remote node updated.|نود خارجی به‌روز شد.
Remove stored bot token|حذف کلید ذخیره‌شده ربات
Require administrator 2FA|الزام احراز هویت دومرحله‌ای مدیران
Reset|بازنشانی
Reset persisted quota usage|بازنشانی مصرف ثبت‌شده سهمیه
Reset usage|بازنشانی مصرف
Reset used traffic automatically.|بازنشانی خودکار مصرف ترافیک.
Restart Xray|راه‌اندازی مجدد هسته
Restart Xray service|راه‌اندازی مجدد سرویس هسته
Review and migrate panel backups|بررسی و انتقال پشتیبان پنل
Review the listener checks below. Resolve conflicting ports in Inbounds, then deploy or restart Xray.|پورت‌های زیر را بررسی کنید. تداخل پورت را در اینباندها رفع و سپس هسته را اجرا یا راه‌اندازی مجدد کنید.
Revoke|لغو کلید
Rewrite address|بازنویسی نشانی
Rewrite port|بازنویسی پورت
Role|نقش
Running|در حال اجرا
Runtime|محیط اجرا
Runtime policy, subscriptions, automation, security and host health.|سیاست اجرا، اشتراک‌ها، خودکارسازی، امنیت و وضعیت میزبان.
SOCKS options|گزینه‌های ساکس
Save & deploy|ذخیره و اجرا
Save administrator|ذخیره مدیر
Save changes|ذخیره تغییرات
Save node|ذخیره نود
Save settings|ذخیره تنظیمات
Save, validate & deploy|ذخیره، اعتبارسنجی و اجرا
Scope|محدوده دسترسی
Scoped API keys|کلیدهای دسترسی محدود
Search audit|جست‌وجوی رویدادها
Search clients...|جست‌وجوی کلاینت‌ها…
Search profiles|جست‌وجوی پروفایل‌ها
Search users|جست‌وجوی کاربران
Security|امنیت
Security & API|امنیت و رابط برنامه‌نویسی
Select a node to preview configuration.|برای پیش‌نمایش پیکربندی یک نود انتخاب کنید.
Select a node.|یک نود انتخاب کنید.
Select all visible clients|انتخاب همه کلاینت‌های نمایش‌داده‌شده
Select inbound|انتخاب اینباند
Select node|انتخاب نود
Select up to 200 clients per operation.|در هر عملیات حداکثر ۲۰۰ کلاینت انتخاب کنید.
Send a test to configured owners|ارسال پیام آزمایشی به مالکان
Send test to owners|ارسال آزمایش به مالکان
Server key pair|جفت کلید سرور
Server management|مدیریت سرور
Server name|نام سرور
Service|سرویس
Service health and configuration|وضعیت سرویس و پیکربندی
Service health, connection configuration and deployment.|وضعیت سرویس، پیکربندی اتصال و اجرا.
Session-authenticated REST endpoints used by the VeloRay web panel and automation.|مسیرهای رابط برنامه‌نویسی با احراز هویت نشست برای پنل و خودکارسازی.
Set credentials, traffic quota and expiry.|اطلاعات اتصال، سهمیه ترافیک و انقضا را تنظیم کنید.
Set the server name and certificate paths, then check the port.|نام سرور و مسیر گواهی را تنظیم و سپس پورت را بررسی کنید.
Set up 2FA|تنظیم احراز هویت دومرحله‌ای
Set up your first connection|راه‌اندازی اولین اتصال
Settings|تنظیمات
Settings are read-only for non-superuser operators.|تنظیمات برای کاربران غیرمدیر فقط خواندنی است.
Settings saved.|تنظیمات ذخیره شد.
Sign out|خروج
Skip to content|رفتن به محتوا
Staff|کاربر پنل
Staff access|دسترسی پنل
Staff and superuser accounts that can sign in to the panel.|حساب‌های دارای دسترسی ورود به پنل.
Status|وضعیت
Stopped|متوقف
Store these offline. Each code works once.|این کدها را آفلاین نگهداری کنید. هر کد یک بار قابل استفاده است.
Stream settings JSON|تنظیمات انتقال به صورت جی‌سان
Subscription|اشتراک
Subscription directory|فهرست اشتراک‌ها
Subscription portal|صفحه اشتراک
Subscription profiles|پروفایل‌های اشتراک
Subscriptions|اشتراک‌ها
Subscriptions enabled|اشتراک‌ها فعال هستند
Superuser|مدیر اصلی
Support URL|نشانی پشتیبانی
Swap|حافظه مجازی
System|سیستم
TLS certificate|گواهی امنیتی
TLS private key|کلید خصوصی امنیتی
TOTP with single-use recovery codes|رمز یک‌بارمصرف و کدهای بازیابی
TUN changes host routing. VeloRay validates the Xray config, but route design should still be tested carefully to avoid loops.|تونل مسیریابی میزبان را تغییر می‌دهد. پیکربندی بررسی می‌شود؛ مسیرها را برای جلوگیری از حلقه آزمایش کنید.
TUN interface|رابط تونل
Target|هدف
Telegram|تلگرام
Telegram automation|خودکارسازی تلگرام
Telegram enabled|تلگرام فعال است
Template|قالب
Test delivered to configured owners.|پیام آزمایشی به مالکان ارسال شد.
The clean URL opens the VeloRay portal in browsers and automatically serves a compatible feed to known VPN clients.|نشانی اصلی در مرورگر صفحه اشتراک و در برنامه‌های شناخته‌شده خروجی سازگار را باز می‌کند.
The full key is displayed only once after creation.|کلید کامل فقط یک بار پس از ساخت نمایش داده می‌شود.
The patch is merged into VeloRay's generated config. The local stats API and traffic counters stay enabled.|تنظیمات تکمیلی با پیکربندی ساخته‌شده ادغام می‌شود. آمار محلی و شمارنده‌های ترافیک فعال می‌مانند.
This page could not load|این صفحه بارگذاری نشد
This protocol is not metered per managed account.|مصرف این پروتکل برای هر حساب محاسبه نمی‌شود.
This secret will not be shown again.|این کلید دوباره نمایش داده نمی‌شود.
Time|زمان
Today|امروز
Token configured|کلید تنظیم‌شده
Top consumers|بیشترین مصرف‌کنندگان
Total|مجموع
Total clients|مجموع کلاینت‌ها
Track usage, quotas and renewal dates.|پیگیری مصرف، سهمیه‌ها و تاریخ تمدید.
Traffic|ترافیک
Traffic and server status|ترافیک و وضعیت سرور
Traffic appears after clients start using the node.|پس از استفاده کلاینت‌ها از نود، ترافیک نمایش داده می‌شود.
Traffic collected during the last 24 hours|ترافیک ثبت‌شده در ۲۴ ساعت گذشته
Traffic history retention|نگهداری تاریخچه ترافیک
Traffic limit (GB)|سقف ترافیک (گیگابایت)
Traffic multiplier|ضریب مصرف
Traffic overview|نمای ترافیک
Transport|انتقال
Trojan TLS|تروجان امن
Tunnel|تونل
Tunnel / port forwarding|تونل و هدایت پورت
Two-factor authentication|احراز هویت دومرحله‌ای
Two-factor authentication disabled.|احراز هویت دومرحله‌ای غیرفعال شد.
Two-factor authentication enabled. Save the recovery codes now.|احراز هویت دومرحله‌ای فعال شد. کدهای بازیابی را ذخیره کنید.
UDP idle timeout|مهلت بی‌کاری یودی‌پی
Unchanged when empty|در صورت خالی بودن تغییر نمی‌کند
Unexpected UI error|خطای غیرمنتظره رابط کاربری
Unique accounts|حساب‌های یکتا
Unknown|نامشخص
Unlimited|نامحدود
Up to 500 latest audit records|حداکثر ۵۰۰ رویداد اخیر
Update or remove inbound|ویرایش یا حذف اینباند
Update policy, including traffic_multiplier|تغییر سیاست و ضریب مصرف
Uptime|مدت کارکرد
Usage above 80%|مصرف بالاتر از ۸۰٪
Usage and quotas update every 5 seconds.|مصرف و سهمیه‌ها هر ۵ ثانیه به‌روز می‌شوند.
Use 0.0.0.0 for IPv4 or :: for IPv6. Enter a specific IP to restrict the listener.|برای همه نشانی‌ها از ۰.۰.۰.۰ یا دو دونقطه استفاده کنید. برای محدودکردن شنود، آی‌پی مشخص وارد کنید.
Use Bearer tokens for automation without sharing a panel password.|برای خودکارسازی بدون اشتراک‌گذاری رمز پنل، از کلید دسترسی استفاده کنید.
Use strong passwords and enable 2FA after first login.|از رمز قوی استفاده کنید و پس از اولین ورود احراز هویت دومرحله‌ای را فعال کنید.
User|کاربر
Username|نام کاربری
Validate|اعتبارسنجی
Validate and apply generated config|اعتبارسنجی و اجرای پیکربندی
Validate configuration and listener availability|اعتبارسنجی پیکربندی و بررسی پورت
VeloRay version|نسخه برنامه
Verify the Agent TLS certificate|بررسی گواهی امنیتی عامل
Version|نسخه
Version pending|نسخه در انتظار دریافت
WG config|پیکربندی وایرگارد
Waiting for local node|در انتظار نود محلی
Within the next 7 days|در ۷ روز آینده
Xray Core|هسته ایکس‌ری
Xray is|وضعیت هسته:
Xray restarted.|هسته راه‌اندازی مجدد شد.
active|فعال
actual ·|واقعی ·
administrator account(s) do not have TOTP enabled yet.|حساب هنوز احراز هویت دومرحله‌ای فعال ندارد.
auto|خودکار
billed|محاسبه‌شده
clients|کلاینت
clients ·|کلاینت ·
days|روز
e.g. Arman - iPhone|مثلاً آرمان - گوشی
gRPC service|سرویس جی‌آرپی‌سی
listeners across|اینباند روی
nodes|نود
not installed|نصب نشده
of|از
on|روشن
online|آنلاین
owners ·|مالک ·
recorded total|مجموع ثبت‌شده
recovery codes remaining|کد بازیابی باقی مانده
registered node|نود ثبت‌شده
s|
selected|انتخاب‌شده
to another node or port.|به نود یا پورت دیگر.
token, selected source IDs and optional port_overrides; save disabled|کلید، شناسه‌های انتخاب‌شده و تغییر اختیاری پورت؛ ذخیره به صورت غیرفعال
used|مصرف‌شده
Reconnecting…|در حال اتصال مجدد…
Choose file|انتخاب فایل
No file selected|فایلی انتخاب نشده است
Change language|تغییر زبان
Dark|تیره
Light|روشن
System theme|تم سیستم
Theme: dark|تم: تیره
Theme: light|تم: روشن
Theme: system|تم: سیستم
administrator|مدیر
reseller|فروشنده
viewer|مشاهده‌گر
custom|سفارشی
`;
export const fa: Record<string, string> = Object.fromEntries(
  entries
    .trim()
    .split("\n")
    .map((line) => line.split("|")),
);
Object.assign(fa, {
  "Checking…": "در حال بررسی…",
  Unavailable: "در دسترس نیست",
  Connected: "متصل",
  "Some system information is unavailable":
    "بخشی از اطلاعات سیستم در دسترس نیست",
  "No outbounds": "خروجی‌ای وجود ندارد",
  Inspect: "مشاهده",
  "Chart start": "شروع بازهٔ نمودار",
  "Chart end": "پایان بازهٔ نمودار",
  "Full range": "کل بازه",
  "{0}s": "{0} ثانیه",

  "IP's": "آی‌پی‌ها",
  "Device's": "دستگاه‌ها",
  "No connected IPs": "آی‌پی متصلی وجود ندارد",
  "No registered devices": "دستگاهی ثبت نشده است",
  "Devices are registered by apps that send HWID.":
    "ثبت دستگاه توسط برنامه‌های دارای HWID انجام می‌شود.",
  "Some connection data is unavailable": "بخشی از اطلاعات اتصال در دسترس نیست",
  "Last observed": "آخرین مشاهده",
  "First registered": "اولین ثبت",
  "Last subscription update": "آخرین دریافت ساب",
  Application: "برنامه",
  Device: "دستگاه",
  "Could not load connections": "دریافت اطلاعات اتصال انجام نشد",

  " · Shared across {0} connections": " · مشترک بین {0} اتصال",
  "Actions for {0}": "عملیات برای {0}",
  "Delete administrator {0}?": "حساب {0} حذف شود؟",
  "Delete inbound {0}?": "اینباند {0} حذف شود؟",
  "Delete node {0}? Remove this node's inbounds first.":
    "نود {0} حذف شود؟ ابتدا اینباندهای آن را حذف کنید.",
  "Delete {0} selected clients?": "{0} کلاینت انتخاب‌شده حذف شوند؟",
  "Delete {0}?": "{0} حذف شود؟",
  "Recorded traffic from {0} to {1}": "ترافیک ثبت‌شده از {0} تا {1}",
  "Restart Xray on {0}? Active connections may briefly reconnect.":
    "هسته روی {0} دوباره راه‌اندازی شود؟ اتصال‌ها ممکن است کوتاه قطع و وصل شوند.",
  "Revoke API key {0}?": "کلید دسترسی {0} لغو شود؟",
  "Select {0}": "انتخاب {0}",
  "Theme: {0}": "تم: {0}",
  "Validated config deployed to {0}.": "پیکربندی معتبر روی {0} اجرا شد.",
  "{0} accounts · {1} actual": "{0} حساب · {1} واقعی",
  "{0} active": "{0} فعال",
  "{0} clients created in one deployment.": "{0} کلاینت در یک عملیات ساخته شد.",
  "{0} clients updated in one deployment per node.":
    "{0} کلاینت با یک عملیات برای هر نود به‌روز شد.",
  "{0} copy": "نسخه {0}",
  "{0} days": "{0} روز",
  "{0} deleted.": "{0} حذف شد.",
  "{0} responded successfully.": "{0} با موفقیت پاسخ داد.",
  "{0} total": "مجموع {0}",
  "{0} unlimited": "{0} نامحدود",
  "{0} validated and deployed.": "{0} اعتبارسنجی و اجرا شد.",
  "{0}% of quotas": "{0}٪ سهمیه‌ها",
  "{0}% · {1} cores": "{0}٪ · {1} هسته",
  "{0}d {1}h": "{0} روز {1} ساعت",
  "{0}h {1}m": "{0} ساعت {1} دقیقه",
  "{0}m": "{0} دقیقه",
  dark: "تیره",
  light: "روشن",
  system: "سیستم",
  ok: "سالم",
  offline: "آفلاین",
  online: "آنلاین",
  unknown: "نامشخص",
  pending: "در انتظار",
  applied: "اجراشده",
  error: "خطا",
  degraded: "نیازمند بررسی",
  rollback_pending: "در انتظار بازیابی",
  stopped: "متوقف",
  running: "در حال اجرا",
  read: "خواندن",
  write: "نوشتن",
  admin: "مدیر",
  disabled: "غیرفعال",
  enabled: "فعال",
  manual: "دستی",
  quota: "سهمیه",
  expired: "منقضی",
  "enabled ·": "فعال ·",
  free: "آزاد",
  conflict: "تداخل",
  active: "فعال",
  none: "بدون",
  raw: "مستقیم",
  tls: "رمزگذاری‌شده",
  "account client limit reached": "سقف تعداد کلاینت‌های حساب پر شده است",
  "account traffic budget exceeded": "بودجه ترافیک حساب کافی نیست",
  "inbound is not assigned to your account":
    "این اینباند به حساب شما اختصاص ندارد",
  "inbound is not assigned to the client owner":
    "این اینباند به مالک کلاینت اختصاص ندارد",
  "a limited reseller account requires finite client quotas":
    "برای حساب فروشنده با بودجه محدود، سهمیه کلاینت باید محدود باشد",
  "client not found": "کلاینت یافت نشد",
  "inbound not found": "اینباند یافت نشد",
  "administrator access required": "دسترسی مدیر لازم است",
  "sign in required": "ورود به حساب لازم است",
  "session expired": "نشست منقضی شده است",
  "invalid username or password": "نام کاربری یا رمز عبور نادرست است",
  "Refresh the page and try again": "صفحه را دوباره بارگذاری کنید",
  "Set up two-factor authentication to continue":
    "برای ادامه احراز هویت دومرحله‌ای را تنظیم کنید",
  "Too many attempts. Try again later":
    "تعداد تلاش‌ها زیاد است. بعداً دوباره امتحان کنید",
  ". Management actions require a confirmation button that expires after five minutes. Every owner must start the bot before testing. Save changed settings before checking the connection.":
    ". عملیات مدیریت نیاز به تأیید دارد و پس از پنج دقیقه منقضی می‌شود. هر مالک باید پیش از آزمایش ربات را شروع کند. پیش از بررسی ارتباط، تنظیمات را ذخیره کنید.",
  "Choose a file": "انتخاب فایل",
  "No backup selected": "فایلی انتخاب نشده است",
});

Object.assign(fa, {
  "0 B": "۰ بایت",
  B: "بایت",
  KB: "کیلوبایت",
  MB: "مگابایت",
  TB: "ترابایت",
  "Protocol options": "گزینه‌های پروتکل",
});
Object.assign(fa, {
  "{0} logical cores": "{0} هسته منطقی",
  "{0} protocol families": "{0} خانواده پروتکل",
  "{0} total · {1} disabled": "مجموع {0} · {1} غیرفعال",
});

Object.assign(fa, { "Account directory": "فهرست حساب‌ها" });
Object.assign(fa, {
  "Pause public subscriptions while administrator access remains online.":
    "توقف اشتراک‌های عمومی با حفظ دسترسی مدیر به پنل.",
  "Turn public subscription endpoints on or off.":
    "فعال یا غیرفعال‌کردن نشانی‌های عمومی اشتراک.",
  "Display compatible client suggestions.": "نمایش پیشنهاد برنامه‌های سازگار.",
  "Allow subscription URL QR display.": "نمایش کد تصویری نشانی اشتراک.",
  "Expose the raw protocol URI when supported.":
    "نمایش نشانی مستقیم پروتکل در صورت پشتیبانی.",
  "Staff must enroll TOTP before protected API use.":
    "کاربران پنل باید پیش از استفاده از رابط محافظت‌شده، رمز یک‌بارمصرف را تنظیم کنند.",
  "Allow scoped Bearer tokens from Accounts.":
    "اجازه کلیدهای دسترسی محدود از بخش حساب‌ها.",
  "Send alerts and accept commands from configured owner IDs.":
    "ارسال هشدار و دریافت دستور از شناسه‌های مالک تنظیم‌شده.",
  "Enable confirmed client and node changes. Required panel 2FA keeps bot management read-only.":
    "فعال‌کردن تغییرات تأییدشده کلاینت و نود. با الزام احراز هویت دومرحله‌ای، مدیریت ربات فقط خواندنی است.",
  "Notify owners before quota exhaustion or account expiry.":
    "اطلاع به مالکان پیش از اتمام سهمیه یا انقضای حساب.",
  "Clear the encrypted Telegram credential on save.":
    "حذف کلید رمزگذاری‌شده تلگرام هنگام ذخیره.",
});
Object.assign(fa, {
  "Verify that the TLS certificate and private-key files exist and are readable on the target node":
    "وجود و دسترسی خواندن فایل گواهی و کلید خصوصی را روی نود مقصد بررسی کنید",
  "Host overrides, remote node credentials, IP limits, fallback chains and calendar reset schedules need review; they are not activated automatically":
    "تغییرات میزبان، اطلاعات نود خارجی، محدودیت آی‌پی، مسیرهای جایگزین و زمان‌بندی بازنشانی نیاز به بررسی دارند و خودکار فعال نمی‌شوند",
  "Fallback chains require manual configuration and are not activated":
    "مسیرهای جایگزین به تنظیم دستی نیاز دارند و فعال نشده‌اند",
  "Live port inspection is unavailable; check the target node before activation":
    "بررسی زنده پورت در دسترس نیست؛ پیش از فعال‌سازی نود مقصد را بررسی کنید",
  "Relational subscription tokens are regenerated; distribute the new VeloRay subscription addresses":
    "کلیدهای اشتراک دوباره ساخته می‌شوند؛ نشانی اشتراک جدید را در اختیار کاربران قرار دهید",
  "Calendar resets, on-hold activation, host overrides and node credentials need review in VeloRay":
    "بازنشانی تقویمی، فعال‌سازی در انتظار، تغییرات میزبان و اطلاعات نود نیاز به بررسی دارند",
  "Non-Xray core configuration was skipped":
    "پیکربندی هسته‌های دیگر نادیده گرفته شد",
  "Source port is invalid": "پورت مبدا معتبر نیست",
  "Inline TLS material must be replaced with target-node certificate files":
    "اطلاعات گواهی درون پیکربندی باید با فایل گواهی نود مقصد جایگزین شود",
  "VLESS encrypted decryption settings require manual migration":
    "تنظیمات رمزگذاری وی‌لس به انتقال دستی نیاز دارد",
  "Client name exceeds 96 characters": "نام کلاینت از ۹۶ نویسه بیشتر است",
  "Negative source traffic counters are not accepted":
    "شمارنده منفی ترافیک مبدا پذیرفته نمی‌شود",
  "WireGuard server key pair generated.": "جفت کلید سرور وایرگارد ساخته شد.",
  "VLESS flow": "حالت جریان VLESS",
  "0.0.0.0/0, ::/0 (optional)": "0.0.0.0/0, ::/0 (اختیاری)",
  s: "ثانیه",
});

Object.assign(fa, {
  Outbounds: "خروجی‌ها",
  Routing: "مسیریابی",
  "New outbound": "خروجی جدید",
  "Configured outbounds": "خروجی‌های تنظیم‌شده",
  "New rule": "قانون جدید",
  Rules: "قوانین",
  Core: "هسته",
  Priority: "اولویت",
  Match: "تطبیق",
  Tag: "برچسب",
  Devices: "دستگاه‌ها",
  "Start after first use": "شروع پس از اولین استفاده",
  "0 = unlimited · concurrent IPs": "۰ = نامحدود · آی‌پی‌های هم‌زمان",
  "days · after first use": "روز · پس از اولین استفاده",
  Used: "مصرف‌شده",
  "Transfer rate": "نرخ انتقال",
  "Traffic chart": "نمودار ترافیک",
  "Select date": "انتخاب تاریخ",
  Clear: "پاک‌کردن",
  Done: "تمام",
  "Scan SNI": "اسکن SNI",
  "SNI scanner": "اسکنر SNI",
  Domain: "دامنه",
  Scan: "اسکن",
  "Find SNI": "پیداکردن SNI",
  "Scanning from this node…": "در حال بررسی از این نود…",
  Use: "انتخاب",
  "Create key": "ساخت کلید",
  "Your API key": "کلید API شما",
  "Copy now. This key is shown only once.":
    "اکنون کپی کنید. این کلید فقط یک بار نمایش داده می‌شود.",
  "Valid for (days)": "اعتبار (روز)",
  "Reset key": "بازنشانی کلید",
  "No keys yet.": "هنوز کلیدی ساخته نشده است.",
  "No entries yet.": "هنوز موردی ساخته نشده است.",
  "Enter a JSON object.": "یک شیء JSON وارد کنید.",
  Create: "ساخت",
});

Object.assign(fa, {
  "Verified GitHub auto-update": "به‌روزرسانی خودکار تأییدشده از گیت‌هاب",
  "Daily commit checks, CI verification, restricted pre-update backup and application rollback.":
    "بررسی روزانه تغییرات، تأیید تست‌ها، تهیه نسخه پشتیبان امن و امکان بازگشت برنامه.",
  "Automatic updates enabled": "به‌روزرسانی خودکار فعال است",
  "Automatic updates disabled": "به‌روزرسانی خودکار غیرفعال است",
  "Tracked repository": "مخزن تحت نظارت",
  "Tracked branch": "شاخه تحت نظارت",
  "Installed GitHub commit": "شناسه نسخه نصب‌شده",
  "Not recorded yet": "هنوز ثبت نشده",
  "Run once on the server to activate automatic updates:":
    "برای فعال‌سازی به‌روزرسانی خودکار، یک بار روی سرور اجرا کنید:",
  "The panel does not execute root commands from the browser. Once enabled, the Linux systemd timer checks GitHub every day. Updates are installed only after that exact commit passes CI.":
    "پنل دستورات مدیریتی سرور را از مرورگر اجرا نمی‌کند. پس از فعال‌سازی، زمان‌بند لینوکس هر روز گیت‌هاب را بررسی می‌کند و فقط نسخه‌ای را نصب می‌کند که تست‌های همان تغییر موفق بوده باشند.",
  "Refresh update status": "به‌روزرسانی وضعیت",
  "View source on GitHub": "مشاهده سورس در گیت‌هاب",
});

Object.assign(fa, {
  "Modern dashboard": "داشبورد مدرن",
  "Classic compact portal": "صفحهٔ کلاسیک فشرده",
  "Subscription update interval (hours)": "فاصلهٔ به‌روزرسانی اشتراک (ساعت)",
  "Default subscription format": "فرمت پیش‌فرض اشتراک",
  "Auto detect": "تشخیص خودکار",
  "Subscription portal theme": "تم صفحهٔ اشتراک",
  "Follow device": "هماهنگ با دستگاه",
  Light: "روشن",
  Dark: "تیره",
  "Traffic chart": "نمودار ترافیک",
  "Display actual historical traffic in the subscription portal.":
    "نمایش تاریخچهٔ واقعی مصرف ترافیک در صفحهٔ اشتراک.",
  "Subscription downloads": "دانلود فایل‌های اشتراک",
  "Show export buttons for supported client configuration formats.":
    "نمایش دکمه‌های دریافت کانفیگ برای فرمت‌های پشتیبانی‌شده.",
});

Object.assign(fa, {
  "Simple subscription page": "صفحهٔ سادهٔ اشتراک",
  "Custom HTML file": "فایل HTML سفارشی",
  "HTML template file": "فایل قالب HTML",
  "Validate template": "بررسی قالب",
  "Template is valid. Save settings to activate it.":
    "قالب معتبر است. برای فعال‌سازی، تنظیمات را ذخیره کنید.",
  "Template validation failed": "بررسی قالب ناموفق بود",
  "Use an HTML file inside /etc/veloray/templates. Changes reload automatically; an invalid file falls back to the simple page.":
    "فایل HTML را در مسیر /etc/veloray/templates قرار دهید. تغییرات خودکار خوانده می‌شوند؛ اگر فایل نامعتبر باشد، صفحهٔ ساده نمایش داده می‌شود.",
  "Subscription device policy": "سیاست دستگاه‌های اشتراک",
  "Default HWID device limit": "سقف پیش‌فرض دستگاه بر اساس HWID",
  "0 = unlimited. A positive limit requires HWID support.":
    "صفر یعنی نامحدود. تعیین سقف نیازمند برنامهٔ سازگار با HWID است.",
  "Require HWID even without a device cap":
    "الزام ارسال HWID حتی بدون سقف دستگاه",
  "Apps without HWID cannot download configurations when enabled. Existing copied configurations are not revoked.":
    "با فعال‌بودن این گزینه، برنامه‌های فاقد HWID نمی‌توانند کانفیگ دریافت کنند. کانفیگ‌های قبلاً کپی‌شده باطل نمی‌شوند.",
  "Client configuration templates": "قالب کانفیگ کلاینت‌ها",
  "Optional JSON objects. Leave blank for built-in output. Generated proxies and outbounds are inserted automatically. Use $VELO_PROXIES inside a Clash group's proxies array.":
    "قالب JSON اختیاری است. برای خروجی داخلی خالی بگذارید. پروکسی‌ها و خروجی‌ها خودکار درج می‌شوند. برای نام پروکسی‌ها در آرایهٔ proxies گروه Clash از $VELO_PROXIES استفاده کنید.",
  "Subscription hosts": "آدرس‌های انتشار اشتراک",
  "Publish an inbound through several addresses without changing its listener. A zero port uses the inbound port. If all hosts for an inbound are disabled, it is hidden from subscriptions.":
    "یک ورودی را بدون تغییر تنظیمات واقعی سرور از چند آدرس منتشر کنید. پورت صفر از پورت ورودی استفاده می‌کند. اگر همهٔ آدرس‌های یک ورودی غیرفعال باشند، آن ورودی در اشتراک نمایش داده نمی‌شود.",
  "Could not load inbounds": "دریافت ورودی‌ها ناموفق بود",
  "New host": "آدرس جدید",
  "Select inbound": "انتخاب ورودی",
  "Public address": "آدرس عمومی",
  "Public port": "پورت عمومی",
  "Host header": "هدر Host",
  "Remove host": "حذف آدرس",
  "Add host": "افزودن آدرس",
  "HWID device limit": "سقف دستگاه بر اساس HWID",
  "0 = panel default · -1 = disabled · requires a compatible app":
    "صفر = پیش‌فرض پنل · منفی یک = غیرفعال · نیازمند برنامهٔ سازگار",
  "Could not update device": "به‌روزرسانی دستگاه ناموفق بود",
  Revoked: "مسدودشده",
  "Allow device": "اجازه به دستگاه",
  "Revoke device": "مسدودکردن دستگاه",
  "Forget device": "حذف سابقهٔ دستگاه",
  "Check subscription": "بررسی اشتراک",
  "Could not check subscription": "بررسی اشتراک ناموفق بود",
  "Checks account policy and export compatibility. This does not test connectivity from a user's network.":
    "وضعیت حساب و سازگاری فرمت خروجی بررسی می‌شود. اتصال از شبکهٔ کاربر آزمایش نمی‌شود.",
  "HWID required": "الزام HWID",
  Limit: "سقف",
  "Custom template unavailable; the simple page will be used.":
    "قالب سفارشی در دسترس نیست؛ صفحهٔ ساده نمایش داده می‌شود.",
  Unavailable: "در دسترس نیست",
  Available: "در دسترس",
  account_inactive: "حساب غیرفعال است یا اعتبار و سهمیهٔ آن تمام شده است.",
  inbound_disabled: "ورودی غیرفعال است.",
  node_disabled: "نود غیرفعال است.",
  host_hidden: "این آدرس در اشتراک پنهان شده است.",
  subscriptions_disabled: "اشتراک‌ها غیرفعال هستند.",
  maintenance: "پنل در حالت نگهداری است.",
  "Filter by service": "فیلتر بر اساس سرویس",
  "All services": "همهٔ سرویس‌ها",
  "Select visible": "انتخاب موارد نمایش‌داده‌شده",
  "Clear selection": "پاک کردن انتخاب",
  "Bulk actions": "عملیات گروهی",
  "Select up to 200 customers. Selection includes hidden rows.":
    "حداکثر ۲۰۰ مشتری انتخاب کنید. ردیف‌های پنهان هم در انتخاب باقی می‌مانند.",
  "This affects every selected customer and all their connections. Changes roll back together if deployment fails.":
    "این عملیات بر همهٔ مشتری‌های انتخاب‌شده و اتصال‌های آن‌ها اعمال می‌شود. اگر اعمال تنظیمات ناموفق باشد، همهٔ تغییرات برمی‌گردند.",
  "Edit quota and access": "ویرایش سهمیه و دسترسی",
  "Replace traffic quota": "جایگزینی سهمیهٔ ترافیک",
  "Replace expiry": "جایگزینی تاریخ انقضا",
  "Empty expiry means no expiration.": "تاریخ خالی یعنی بدون انقضا.",
  "Replace service access": "جایگزینی دسترسی سرویس‌ها",
  "Direct inbounds and usage are preserved. Plan defaults are not reapplied.":
    "ورودی‌های مستقیم و مصرف حفظ می‌شوند. پیش‌فرض‌های پلن دوباره اعمال نمی‌شوند.",
  "Current usage will be reset. Lifetime traffic is preserved.":
    "مصرف دورهٔ جاری صفر می‌شود. مصرف کل حفظ می‌شود.",
  "Selected customers, their connections and subscription URLs will be deleted.":
    "مشتری‌های انتخاب‌شده، اتصال‌ها و لینک‌های اشتراک آن‌ها حذف می‌شوند.",
  "Apply to selected": "اعمال بر موارد انتخاب‌شده",
  "Outbound traffic": "ترافیک خروجی‌ها",
  "Independent raw counters per core and tag. These totals do not consume customer quota and may include several hops.":
    "شمارندهٔ خام و مستقل برای هر هسته و برچسب. این اعداد از سهمیهٔ مشتری کم نمی‌شوند و ممکن است چند مسیر متوالی را شامل شوند.",
  "Traffic period": "بازهٔ ترافیک",
  "Refresh traffic": "تازه‌سازی ترافیک",
  "Current outbound telemetry is unavailable. Upgrade the node agent and apply its configuration.":
    "آمار فعلی خروجی‌ها در دسترس نیست. عامل نود را به‌روز و تنظیمات آن را اعمال کنید.",
  "Last collected": "آخرین دریافت",
  "Period upload (GiB)": "آپلود بازه (GiB)",
  "Period download (GiB)": "دانلود بازه (GiB)",
  "Tracked upload (GiB)": "آپلود ثبت‌شده (GiB)",
  "Tracked download (GiB)": "دانلود ثبت‌شده (GiB)",
  "No outbound traffic recorded yet.":
    "هنوز ترافیکی برای خروجی‌ها ثبت نشده است.",
  "Tracked totals start with the first durable agent sample; period history follows traffic retention. Reusing a tag continues its totals.":
    "جمع ثبت‌شده از نخستین نمونهٔ پایدار عامل شروع می‌شود؛ سابقهٔ بازه تابع مدت نگهداری ترافیک است. استفادهٔ مجدد از برچسب، جمع قبلی را ادامه می‌دهد.",
  "Next hop (optional)": "گام بعدی (اختیاری)",
  "Connect directly": "اتصال مستقیم",
  "Dial this outbound through another outbound. Recursive chains are rejected.":
    "این خروجی از طریق خروجی دیگری متصل می‌شود. زنجیره‌های حلقوی پذیرفته نمی‌شوند.",
  "Default candidate": "گزینهٔ پیش‌فرض",
  "First candidate": "نخستین گزینه",
  "Probe URL": "نشانی آزمون",
  "Probe interval": "فاصلهٔ آزمون‌ها",
  "Latency tolerance (ms)": "تحمل اختلاف تأخیر (میلی‌ثانیه)",
  "Probe idle timeout": "مهلت بی‌کاری آزمون",
  "Interrupt existing connections when switching":
    "قطع اتصال‌های موجود هنگام تعویض مسیر",

  "Subscription policies": "سیاست‌های اشتراک",
  "Subscription policy": "سیاست اشتراک",
  "Add policy": "افزودن سیاست",
  "Combine local connections with external links. Policies can override export templates and defaults for assigned customers, services and apps.":
    "اتصال‌های محلی را با لینک‌های خارجی ترکیب کنید. سیاست‌ها قالب و فرمت پیش‌فرض را برای مشتری‌ها، سرویس‌ها و برنامه‌های انتخاب‌شده تعیین می‌کنند.",
  "Cached connections": "اتصال ذخیره‌شده",
  "Refresh source": "تازه‌سازی منبع",
  "External feed URL": "نشانی ساب خارجی",
  "External connection links": "لینک‌های اتصال خارجی",
  "Public HTTPS only. Refresh explicitly after saving; failed refreshes retain the previous cache.":
    "فقط نشانی عمومی HTTPS. پس از ذخیره منبع را تازه‌سازی کنید؛ در صورت خطا، نسخهٔ قبلی حفظ می‌شود.",
  "Client applications": "برنامه‌های مصرف‌کننده",
  "No app selection applies to every app. Application matching controls presentation, not authorization.":
    "اگر برنامه‌ای انتخاب نکنید، سیاست برای همه اعمال می‌شود. انتخاب برنامه فقط نحوهٔ نمایش و خروجی را تعیین می‌کند.",
  "Client templates": "قالب‌های خروجی",
  "External traffic and expiry are enforced by the external provider. Local account eligibility and HWID checks still protect this subscription.":
    "مصرف و انقضای اتصال خارجی با ارائه‌دهندهٔ آن است. اعتبار حساب محلی و محدودیت دستگاه همچنان بررسی می‌شود.",
  "Notification destinations": "مقصدهای اعلان",
  "Notification destination": "مقصد اعلان",
  "Add destination": "افزودن مقصد",
  "Route events independently to signed HTTPS endpoints or Discord. Empty filters receive all events; use client.*, node.* or an exact event name.":
    "رویدادها را جداگانه به وبهوک امضاشده یا دیسکورد بفرستید. فیلتر خالی همهٔ رویدادها را می‌گیرد؛ از client.*، node.* یا نام دقیق رویداد استفاده کنید.",
  "Signed webhook": "وبهوک امضاشده",
  Channel: "کانال",
  "Event filters": "فیلتر رویدادها",
  "Delivery queued": "ارسال در صف قرار گرفت",
  "Pending deliveries for this destination will also be removed.":
    "ارسال‌های در انتظار این مقصد هم حذف می‌شوند.",
  "Date display": "نمایش تاریخ",
  Calendar: "تقویم",
  "Language default": "پیش‌فرض زبان",
  "Persian (Jalali)": "فارسی (شمسی)",
  Gregorian: "میلادی",
  "Display timezone": "منطقهٔ زمانی نمایش",
  "Date inputs keep their labeled timezone and Gregorian format. Display preferences are saved for this browser.":
    "ورود تاریخ با قالب میلادی و منطقهٔ زمانی درج‌شده در فیلد انجام می‌شود. ترجیح نمایش در همین مرورگر ذخیره می‌شود.",
  "Save date preferences": "ذخیرهٔ نمایش تاریخ",
  "Enter a valid IANA timezone.":
    "نام معتبر منطقهٔ زمانی وارد کنید؛ مثلاً Asia/Tehran.",
  "OIDC issuer (optional)": "نشانی صادرکنندهٔ OIDC (اختیاری)",
  "OIDC subject (optional)": "شناسهٔ حساب OIDC (اختیاری)",
});

Object.assign(fa, {
  Operations: "عملیات",
  "Outbound health, node costs and deployment history.":
    "پایش خروجی‌ها، هزینهٔ نود و تاریخچهٔ استقرار.",
  "Health monitors": "پایش سلامت",
  "Node billing": "هزینه و مصرف نود",
  Diagnostics: "عیب‌یابی",
  "Deployment diagnostics": "عیب‌یابی استقرار",
  "Hosting provider": "ارائه‌دهندهٔ سرور",
  Region: "منطقه",
  "Account reference": "شناسهٔ قرارداد",
  Currency: "واحد پول",
  "Billing timezone": "منطقهٔ زمانی صورتحساب",
  "Price in minor units": "هزینه به کوچک‌ترین واحد پول",
  "Traffic budget (GiB)": "سقف مصرف نود (گیبی‌بایت)",
  "Warning threshold (%)": "آستانهٔ هشدار (درصد)",
  "Monthly reset day": "روز شروع دورهٔ ماهانه",
  "Current cycle": "دورهٔ جاری",
  "Tracked lifetime": "کل مصرف ثبت‌شده",
  "Save billing": "ذخیرهٔ تنظیمات هزینه",
  "Export billing history": "خروجی تاریخچهٔ مصرف",
  "Track node billing": "ثبت دوره‌های مصرف نود",
  "Billing settings saved": "تنظیمات هزینه ذخیره شد",
  "Search monitors": "جست‌وجوی پایشگرها",
  "Add monitor": "افزودن پایشگر",
  "Health monitor": "پایشگر سلامت",
  "Monitor saved": "پایشگر ذخیره شد",
  "No monitors configured": "پایشگری تنظیم نشده است",
  "Run probe": "اجرای بررسی",
  "Probe succeeded": "بررسی موفق بود",
  "Probe failed": "بررسی ناموفق بود",
  Healthy: "سالم",
  Down: "قطع",
  Pending: "در انتظار",
  History: "تاریخچه",
  "Delete monitor": "حذف پایشگر",
  "Confirm delete": "تأیید حذف",
  "Probe history": "تاریخچهٔ بررسی",
  "Export history": "خروجی تاریخچه",
  "HTTPS probe URL": "نشانی HTTPS برای بررسی",
  "Outbound tag": "برچسب خروجی",
  "Check interval (seconds)": "فاصلهٔ بررسی‌ها (ثانیه)",
  "Failures before alert": "تعداد خطا پیش از هشدار",
  "Save monitor": "ذخیرهٔ پایشگر",
  "Email delivery and lifecycle alerts": "ارسال ایمیل و هشدارهای حساب",
  "Enable SMTP email": "فعال‌کردن ارسال ایمیل SMTP",
  "SMTP hostname": "نام میزبان SMTP",
  "SMTP port": "پورت SMTP",
  Encryption: "رمزنگاری",
  "Sender email": "ایمیل فرستنده",
  "SMTP username": "نام کاربری SMTP",
  "SMTP password": "رمز SMTP",
  "Clear SMTP password": "حذف رمز ذخیره‌شدهٔ SMTP",
  "Quota warning (%)": "هشدار مصرف (درصد)",
  "Expiry warning (days)": "هشدار انقضا (روز)",
  "Inactive warning (days)": "هشدار عدم فعالیت (روز)",
  Email: "ایمیل",
  "Recipient emails": "ایمیل گیرندگان",
  "Choose date": "انتخاب تاریخ",
  "Previous month": "ماه قبل",
  "Next month": "ماه بعد",
  "Browser timezone": "منطقهٔ زمانی مرورگر",
  "Only verified TLS is allowed. Save settings, then add an Email destination below. Passwords are encrypted and never returned.":
    "اتصال فقط با گواهی معتبر TLS انجام می‌شود. ابتدا تنظیمات را ذخیره و سپس یک مقصد ایمیلی اضافه کنید. رمزها رمزنگاری می‌شوند و در پاسخ نمایش داده نمی‌شوند.",
  "Zero disables an alert. Events are deduplicated per shared account and delivered through matching notification destinations.":
    "صفر یعنی غیرفعال. برای هر حساب مشترک از ارسال تکراری رویداد جلوگیری می‌شود و رویدادها به مقصدهای منطبق فرستاده می‌شوند.",
  "Probes use the selected outbound in an isolated core process. Failure alerts do not restart production traffic.":
    "بررسی با خروجی انتخاب‌شده و در فرایند جداگانهٔ هسته انجام می‌شود. هشدار خطا، اتصال‌های اصلی را ری‌استارت نمی‌کند.",
  "Counts tracked VPN traffic without customer multipliers. This is an estimate, not your provider invoice. Budgets send alerts without disabling a node.":
    "مصرف ثبت‌شدهٔ VPN بدون ضریب مشتری محاسبه می‌شود. این عدد برآورد پنل است و جای صورتحساب ارائه‌دهنده را نمی‌گیرد. عبور از سقف فقط هشدار می‌دهد و نود را قطع نمی‌کند.",
  "Shows runtime status and recent operation IDs, states and hashes. Credentials and full configurations are excluded.":
    "وضعیت هسته و شناسه، وضعیت و هش عملیات اخیر را نشان می‌دهد. اطلاعات ورود و پیکربندی کامل در این گزارش نیست.",
  "Date pickers follow the selected calendar and keep Gregorian API values. Date and time inputs use the browser timezone. Display preferences are saved for this browser.":
    "انتخابگر تاریخ از تقویم انتخاب‌شده استفاده می‌کند؛ مقدار ارسالی به API میلادی است. ورود تاریخ و ساعت بر پایهٔ منطقهٔ زمانی مرورگر انجام می‌شود. تنظیمات نمایش برای همین مرورگر ذخیره می‌شوند.",
});
