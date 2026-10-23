export const expansionFA: Record<string, string> = {
  "insufficient wallet credit": "اعتبار کیف پول کافی نیست.",
  "credit limit is below the outstanding debt":
    "سقف اعتبار از بدهی فعلی کمتر است.",
  "wallet balance would exceed its limit":
    "موجودی کیف پول از سقف مجاز بیشتر می‌شود.",
  "reference was already used for a different entry":
    "این شناسه برای تراکنشی با مشخصات متفاوت استفاده شده است.",
  "purchase reference was already used with different details":
    "این شناسهٔ خرید قبلاً با مشخصات دیگری استفاده شده است.",
  "unique purchase reference required": "شناسهٔ یکتای خرید لازم است.",
  "unique transfer reference required": "شناسهٔ یکتای انتقال لازم است.",
  "invalid ledger entry": "مشخصات تراکنش معتبر نیست.",
  "nonzero whole amount required": "مبلغ باید عدد صحیح و غیرصفر باشد.",
  "unsupported currency": "این واحد پول پشتیبانی نمی‌شود.",
  "gateway supports IRR and IRT only":
    "درگاه فقط از ریال و تومان پشتیبانی می‌کند.",
  "payment gateway supports IRR and IRT":
    "درگاه پرداخت از ریال و تومان پشتیبانی می‌کند.",
  "online payments are not enabled": "پرداخت آنلاین هنوز فعال نشده است.",
  "payment merchant is not configured": "شناسهٔ پذیرندهٔ درگاه تنظیم نشده است.",
  "configure a merchant before enabling payments":
    "پیش از فعال‌سازی پرداخت، شناسهٔ پذیرنده را تنظیم کنید.",
  "merchant_id must be a UUID": "شناسهٔ پذیرنده باید در قالب UUID باشد.",
  "payments require HTTPS": "پرداخت به HTTPS نیاز دارد.",
  "payments require a public HTTPS panel URL":
    "برای پرداخت، آدرس عمومی پنل باید HTTPS باشد.",
  "checkout expired; create a new order":
    "مهلت پرداخت تمام شده است؛ سفارش جدید بسازید.",
  "payment provider is unavailable; try verification again before creating another order":
    "درگاه در دسترس نیست. پیش از ساخت سفارش جدید، وضعیت پرداخت همین سفارش را دوباره بررسی کنید.",
  "payment provider rejected the request": "درگاه درخواست را نپذیرفت.",
  "payment has not been verified": "پرداخت هنوز تأیید نشده است.",
  "invalid payment provider response": "پاسخ درگاه معتبر نیست.",
  "invalid payment authority": "شناسهٔ پرداخت معتبر نیست.",
  "payment credentials are unavailable":
    "اطلاعات لازم برای تأیید پرداخت در دسترس نیست.",
  "payment not found": "پرداخت پیدا نشد.",
  "order not found": "سفارش پیدا نشد.",
  "order has not been paid": "این سفارش هنوز پرداخت نشده است.",
  "order is not awaiting payment": "این سفارش در انتظار پرداخت نیست.",
  "order has no pending payment": "این سفارش پرداخت در انتظار تأیید ندارد.",
  "order is not pending": "سفارش در وضعیت انتظار نیست.",
  "offer is unavailable": "این بسته فعلاً قابل خرید نیست.",
  "offer not found": "بستهٔ فروش پیدا نشد.",
  "customer owner changed; review the order":
    "مالک مشتری تغییر کرده است؛ سفارش را بررسی کنید.",
  "renewal customer was deleted; review this paid order":
    "مشتری این تمدید حذف شده است؛ سفارش پرداخت‌شده را بررسی کنید.",
  "valid customer subscription token required":
    "شناسهٔ معتبر ساب مشتری لازم است.",
  "renewal offer must use the customer's existing service":
    "بستهٔ تمدید باید مربوط به سرویس فعلی مشتری باشد.",
  "wallet renewal is limited to your customers":
    "تمدید با کیف پول فقط برای مشتریان خودتان مجاز است.",
  "traffic top-up requires a finite traffic grant":
    "برای افزایش حجم، بسته باید حجم مشخص داشته باشد.",
  "test payments never create wallet credit or provision customers":
    "پرداخت آزمایشی اعتبار واقعی یا حساب مشتری ایجاد نمی‌کند.",
  "reseller delegation is not enabled":
    "اجازهٔ مدیریت ریسلر زیرمجموعه فعال نیست.",
  "subreseller limit reached": "سقف تعداد ریسلر زیرمجموعه تکمیل شده است.",
  "direct subreseller not found": "ریسلر زیرمجموعهٔ مستقیم پیدا نشد.",
  "subreseller is inactive": "ریسلر زیرمجموعه غیرفعال است.",
  "reseller not found": "ریسلر پیدا نشد.",
  "a subreseller cannot have administrator privileges":
    "ریسلر زیرمجموعه نمی‌تواند دسترسی مدیر کل داشته باشد.",
  "reseller parent cannot be reassigned after creation":
    "والد ریسلر بعد از ایجاد حساب قابل تغییر نیست.",
  "invalid reseller hierarchy": "ساختار ریسلرها معتبر نیست.",
  "reseller hierarchy depth or cycle limit reached":
    "عمق مجاز زیرمجموعه‌ها رد شده یا ساختار حلقه دارد.",
  "parent reseller account is inactive": "حساب ریسلر والد غیرفعال است.",
  "parent reseller is unavailable": "ریسلر والد در دسترس نیست.",
  "subreseller inbounds must be assigned to its parent":
    "اینباندهای زیرمجموعه باید در دسترسی والد هم باشند.",
  "limited parents cannot contain unlimited subresellers":
    "ریسلر محدود نمی‌تواند زیرمجموعه با سهمیهٔ نامحدود داشته باشد.",
  "subreseller client reservation exceeds the parent's available limit":
    "سهمیهٔ مشتری زیرمجموعه از ظرفیت آزاد والد بیشتر است.",
  "subreseller traffic reservation exceeds the parent's available limit":
    "سهمیهٔ ترافیک زیرمجموعه از ظرفیت آزاد والد بیشتر است.",
  "account limit is below its existing clients and child reservations":
    "سقف حساب از مشتریان فعلی و سهمیهٔ رزروشدهٔ زیرمجموعه‌ها کمتر است.",
  "traffic budget is below existing allocations and child reservations":
    "بودجهٔ ترافیک از سهمیه‌های فعلی و رزرو زیرمجموعه‌ها کمتر است.",
  "subreseller limit is below existing direct subresellers":
    "سقف جدید از تعداد زیرمجموعه‌های مستقیم فعلی کمتر است.",
  "remove child inbound assignments before removing them from the parent":
    "پیش از حذف دسترسی اینباند از والد، تخصیص آن به زیرمجموعه‌ها را تغییر دهید.",
  "administrator API scope required to manage reseller identities":
    "مدیریت حساب ریسلر به اسکوپ admin در API نیاز دارد.",
  "max_subresellers must be 0–100": "سقف ریسلر مستقیم باید بین صفر و ۱۰۰ باشد.",
  "seller access denied": "دسترسی این فروشنده به شما واگذار نشده است.",
  "seller is unavailable": "فروشنده فعلاً در دسترس نیست.",
  "store not found": "فروشگاه پیدا نشد.",
  "invalid branding URL or description": "لینک یا توضیحات برندینگ معتبر نیست.",
  "subscription origin must be an HTTPS domain origin":
    "آدرس ساب باید دامنهٔ HTTPS، با پورت اختیاری و بدون مسیر باشد.",
  "subscription origin requires a DNS domain":
    "برای آدرس اختصاصی ساب، دامنهٔ DNS وارد کنید.",
  "invalid subscription origin port": "پورت آدرس اختصاصی ساب معتبر نیست.",
  "DNS verification record does not match this store":
    "رکورد تأیید DNS با این فروشگاه مطابقت ندارد.",
  "DNS verification record is unavailable": "رکورد تأیید DNS در دسترس نیست.",
  "save a subscription origin before verifying DNS":
    "پیش از تأیید DNS، آدرس اختصاصی ساب را ذخیره کنید.",
  "save changed store settings before verifying DNS":
    "ابتدا تغییرات فروشگاه را ذخیره و سپس دامنه را تأیید کنید.",
  "store domain changed; verify the new record":
    "دامنه تغییر کرده است؛ رکورد جدید را تأیید کنید.",
  "this subscription origin belongs to another store":
    "این آدرس ساب قبلاً برای فروشگاه دیگری ثبت شده است.",
  "subscription domain HTTPS is not ready; configure its certificate and proxy first":
    "HTTPS دامنهٔ ساب آماده نیست؛ ابتدا گواهی و پروکسی آن را تنظیم کنید.",
  "subscription domain does not reach this VeloRay instance":
    "این دامنه به همین نمونهٔ VeloRay متصل نیست.",
  "subscription domain health check failed":
    "بررسی سلامت دامنهٔ ساب ناموفق بود.",
  "annual freeze allowance is 30 days": "سهمیهٔ فریز سالانه ۳۰ روز است.",
  "freeze duration must be 1–30 days": "مدت فریز باید بین ۱ تا ۳۰ روز باشد.",
  "only an active subscription with an expiry can be frozen":
    "فقط اشتراک فعال با تاریخ انقضا قابل فریز است.",
  "subscription is already frozen": "اشتراک از قبل فریز شده است.",
  "subscription is not frozen": "این اشتراک فریز نیست.",
  "resume the subscription before renewing":
    "پیش از تمدید، اشتراک را از حالت فریز خارج کنید.",
  "resume the frozen subscription before changing its duration or renewal policy":
    "پیش از تغییر زمان یا برنامهٔ تمدید، اشتراک را از فریز خارج کنید.",
  "resume the frozen subscription before changing its active status":
    "پیش از تغییر وضعیت فعالیت، اشتراک را از فریز خارج کنید.",
  "service membership changed; refresh the impact preview":
    "عضویت سرویس تغییر کرده است؛ پیش‌نمایش اثر تغییرات را تازه‌سازی کنید.",
  "this change would leave a customer without connections":
    "این تغییر، مشتری را بدون اتصال باقی می‌گذارد.",
  "enrollment is expired, used or unavailable":
    "دستور اتصال منقضی، مصرف‌شده یا نامعتبر است.",
  "enrollment issuer is no longer an administrator":
    "صادرکنندهٔ دستور دیگر مدیر کل فعال نیست.",
  "node enrollment requires a public HTTPS panel URL":
    "اتصال خودکار نود به آدرس عمومی HTTPS پنل نیاز دارد.",
  "agent verification failed; check HTTPS reachability, credentials and certificate":
    "تأیید نود ناموفق بود؛ دسترسی HTTPS، اطلاعات اتصال و گواهی را بررسی کنید.",
  "agent identity or core health does not match the enrollment":
    "هویت نود یا سلامت هسته با دستور اتصال مطابقت ندارد.",
  "agent URL must use the enrollment's HTTPS public host":
    "آدرس HTTPS نود باید از همان هاست ثبت‌شده در دستور استفاده کند.",
  "View sales and wallet": "مشاهدهٔ فروش و کیف پول",
  "Manage offers and wallet purchases": "مدیریت بسته‌ها و خرید با کیف پول",
  "View subresellers": "مشاهدهٔ ریسلرهای زیرمجموعه",
  "Manage subresellers": "مدیریت ریسلرهای زیرمجموعه",
};
