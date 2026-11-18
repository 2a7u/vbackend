package main

func GetHead() string {
	return `<!DOCTYPE html>
<html lang="ar" dir="rtl">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
  <title>فينوس — لوحة التحكم</title>
  <link rel="icon" type="image/png" href="/assets/appicon.png">
  
  <!-- Fonts -->
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Cairo:wght@400;500;600;700;800;900&display=swap" rel="stylesheet">
  
  <!-- FontAwesome Icons -->
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  
  <!-- Tailwind CSS via CDN -->
  <script src="https://cdn.tailwindcss.com"></script>
  <script>
    tailwind.config = {
      darkMode: 'class',
      theme: {
        extend: {
          colors: {
            brand: {
              DEFAULT: '#0058BC',
              dark: '#004799',
              light: '#EBF3FF',
              surface: '#FAF9FE',
              accent: '#3B82F6',
            }
          },
          fontFamily: {
            sans: ['Cairo', 'sans-serif'],
          }
        }
      }
    }
  </script>

  <style>
    * { font-family: 'Cairo', sans-serif; }
    ::-webkit-scrollbar { width: 6px; height: 6px; }
    ::-webkit-scrollbar-track { background: #f1f5f9; }
    ::-webkit-scrollbar-thumb { background: #cbd5e1; border-radius: 9999px; }
    ::-webkit-scrollbar-thumb:hover { background: #94a3b8; }
    .modal-backdrop { background-color: rgba(15, 23, 42, 0.6); backdrop-filter: blur(4px); }
    @media print {
      body * { visibility: hidden; }
      #modal-statement, #modal-statement * { visibility: visible; }
      #modal-statement { position: absolute; left: 0; top: 0; width: 100%; max-height: none; border: none; box-shadow: none; padding: 0; }
    }
  </style>
</head>
`
}

func GetLoginView() string {
	return `  <!-- LOGIN VIEW -->
  <div id="view-login" class="relative min-h-screen overflow-hidden bg-slate-50 flex items-center justify-center p-4 sm:p-6">
    <!-- Subtle Modern Ambient Background -->
    <div class="pointer-events-none fixed inset-0 overflow-hidden">
      <div class="absolute -top-32 -right-32 w-96 h-96 rounded-full bg-brand/10 blur-3xl"></div>
      <div class="absolute -bottom-32 -left-32 w-96 h-96 rounded-full bg-sky-400/10 blur-3xl"></div>
    </div>

    <div class="relative w-full max-w-[420px] bg-white/95 backdrop-blur-xl rounded-[28px] shadow-[0_24px_60px_-15px_rgba(0,88,188,0.12),0_4px_20px_-4px_rgba(15,23,42,0.05)] border border-white ring-1 ring-slate-200/70 p-8 sm:p-10 space-y-7">
      <!-- Brand Header -->
      <div class="text-center space-y-3">
        <div class="inline-flex items-center justify-center w-20 h-20 rounded-[22px] bg-gradient-to-b from-white to-slate-50 p-2.5 shadow-lg shadow-brand/10 ring-1 ring-slate-100 mx-auto">
          <img src="/assets/appicon.png" class="w-full h-full object-contain rounded-xl" alt="فينوس">
        </div>
        <div class="space-y-1.5">
          <h1 class="text-2xl sm:text-[26px] font-black text-slate-900 tracking-tight">فينوس</h1>
          <div>
            <span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full bg-brand/5 border border-brand/10 text-brand text-xs font-bold">
              <span class="w-1.5 h-1.5 rounded-full bg-brand"></span>
              <span>لوحة التحكم</span>
            </span>
          </div>
        </div>
      </div>

      <div id="login-error" class="hidden p-3.5 rounded-2xl bg-rose-50 border border-rose-200/80 text-rose-700 text-xs font-bold text-center"></div>

      <form onsubmit="handleLogin(event)" class="space-y-4">
        <div class="group">
          <label for="login-email" class="block text-[13px] font-bold text-slate-700 mb-2">البريد الإلكتروني</label>
          <div class="relative">
            <span class="absolute inset-y-0 right-0 flex items-center pr-4 text-slate-400 group-focus-within:text-brand transition-colors pointer-events-none">
              <i class="fa-regular fa-envelope text-sm"></i>
            </span>
            <input type="email" id="login-email" required autocomplete="email" placeholder="البريد الإلكتروني" class="w-full pr-11 pl-4 py-3.5 bg-slate-50/80 hover:bg-slate-50 focus:bg-white border border-slate-200/90 rounded-2xl text-sm font-semibold text-slate-800 placeholder:text-slate-400 placeholder:font-medium outline-none focus:border-brand focus:ring-4 focus:ring-brand/10 transition-all duration-200">
          </div>
        </div>

        <div class="group">
          <label for="login-password" class="block text-[13px] font-bold text-slate-700 mb-2">كلمة المرور</label>
          <div class="relative">
            <span class="absolute inset-y-0 right-0 flex items-center pr-4 text-slate-400 group-focus-within:text-brand transition-colors pointer-events-none">
              <i class="fa-solid fa-lock text-sm"></i>
            </span>
            <input type="password" id="login-password" required autocomplete="current-password" placeholder="••••••••" class="w-full pr-11 pl-11 py-3.5 bg-slate-50/80 hover:bg-slate-50 focus:bg-white border border-slate-200/90 rounded-2xl text-sm font-semibold text-slate-800 placeholder:text-slate-400 outline-none focus:border-brand focus:ring-4 focus:ring-brand/10 transition-all duration-200">
            <button type="button" onclick="toggleLoginPassword()" aria-label="إظهار كلمة المرور" class="absolute inset-y-0 left-0 flex items-center pl-4 text-slate-400 hover:text-slate-600 transition-colors focus:outline-none">
              <i id="login-password-eye" class="fa-regular fa-eye text-sm"></i>
            </button>
          </div>
        </div>

        <button type="submit" id="btn-login" class="w-full py-3.5 mt-2 bg-gradient-to-l from-brand to-[#006CE6] hover:from-[#004799] hover:to-brand text-white rounded-2xl font-extrabold text-sm shadow-lg shadow-brand/25 hover:shadow-xl hover:shadow-brand/30 hover:-translate-y-0.5 active:translate-y-0 active:scale-[0.99] transition-all duration-200 flex items-center justify-center gap-2.5 disabled:opacity-70 disabled:cursor-not-allowed disabled:transform-none">
          <span id="btn-login-text">تسجيل الدخول</span>
          <i id="btn-login-icon" class="fa-solid fa-arrow-left text-xs"></i>
        </button>
      </form>
    </div>
  </div>
`
}

func GetAppViewStart() string {
	return `  <!-- MAIN CONSOLE APP -->
  <div id="view-app" class="hidden min-h-screen bg-brand-surface flex flex-col">
    <div class="flex-1 flex overflow-hidden">
`
}

func GetSidebarDesktop() string {
	return `      <!-- DESKTOP SIDEBAR -->
      <aside class="hidden lg:flex flex-col w-64 bg-white border-l border-slate-200/80 p-4 space-y-4 select-none flex-shrink-0">
        <!-- Logo & Brand Header -->
        <div class="flex items-center gap-3 px-2 py-1">
          <div class="w-10 h-10 rounded-2xl overflow-hidden bg-brand-light flex items-center justify-center shadow-md shadow-brand/20 flex-shrink-0">
            <img src="/assets/appicon.png" class="w-full h-full object-contain" alt="فينوس">
          </div>
          <div>
            <span class="font-black text-slate-900 text-sm tracking-wide block">فينوس</span>
            <span class="text-[10px] text-slate-400 font-bold block">لوحة التحكم</span>
          </div>
        </div>

        <div class="h-px bg-slate-100 my-1"></div>

        <!-- Navigation Tabs Container -->
        <nav class="flex-1 space-y-4 overflow-y-auto pr-1">
          <!-- GROUP 1: MAIN NAVIGATION -->
          <div class="space-y-1">
            <div class="px-3 text-[10px] font-black text-slate-400 uppercase tracking-wider">القسم الرئيسي</div>
            <button onclick="switchTab('stats')" id="nav-stats" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-chart-pie w-4 text-center text-slate-400"></i><span>الرئيسية</span>
            </button>
            <button onclick="switchTab('users')" id="nav-users" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-users w-4 text-center text-slate-400"></i><span>المشتركين</span>
            </button>
            <button onclick="switchTab('activation')" id="nav-activation" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-key w-4 text-center text-slate-400"></i><span>الباقات</span>
            </button>
            <button onclick="switchTab('wallet')" id="nav-wallet" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-wallet w-4 text-center text-slate-400"></i><span>المحافظ</span>
            </button>
            <button onclick="switchTab('cards')" id="nav-cards" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-sim-card w-4 text-center text-slate-400"></i><span>البطاقات</span>
            </button>
            <button onclick="switchTab('appconfig')" id="nav-appconfig" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-sliders w-4 text-center text-slate-400"></i><span>إعدادات المنصة</span>
            </button>
            <button onclick="switchTab('security')" id="nav-security" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-shield w-4 text-center text-slate-400"></i><span>الرقابة والأمان</span>
            </button>
          </div>

          <!-- GROUP 2: FINTECH CORE -->
          <div class="space-y-1">
            <div class="px-3 text-[10px] font-black text-slate-400 uppercase tracking-wider flex items-center justify-between">
              <span>العمليات المالية</span>
              <span class="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
            </div>
            <button onclick="switchTab('ledger')" id="nav-ledger" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-book-journal-whills w-4 text-center text-slate-400"></i><span>السجل المالي</span>
            </button>
            <button onclick="switchTab('fraud')" id="nav-fraud" class="nav-tab-btn w-full flex items-center justify-between px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <div class="flex items-center gap-3">
                <i class="fa-solid fa-shield-halved w-4 text-center text-slate-400"></i><span>الحماية</span>
              </div>
              <span id="badge-fraud-alerts" class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600 hidden">0</span>
            </button>
            <button onclick="switchTab('reconcile')" id="nav-reconcile" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-scale-balanced w-4 text-center text-slate-400"></i><span>التسويات</span>
            </button>
            <button onclick="switchTab('debts')" id="nav-debts" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-hand-holding-dollar w-4 text-center text-slate-400"></i><span>الائتمان</span>
            </button>
            <button onclick="switchTab('close')" id="nav-close" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-file-invoice-dollar w-4 text-center text-slate-400"></i><span>كشوف الحساب</span>
            </button>
            <button onclick="switchTab('pricing')" id="nav-pricing" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-tags w-4 text-center text-slate-400"></i><span>الأسعار</span>
            </button>
            <button onclick="switchTab('radar')" id="nav-radar" class="nav-tab-btn w-full flex items-center justify-between px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <div class="flex items-center gap-3">
                <i class="fa-solid fa-tower-broadcast w-4 text-center text-slate-400"></i><span>مؤشر النفاد</span>
              </div>
              <span id="badge-radar-low" class="px-2 py-0.5 rounded-full text-[10px] font-black bg-rose-50 text-rose-600 border border-rose-200 hidden">حرج</span>
            </button>
          </div>

          <!-- GROUP 3: OPERATIONS & AUDIT -->
          <div class="space-y-1">
            <div class="px-3 text-[10px] font-black text-slate-400 uppercase tracking-wider">الأدوات التشغيلية</div>
            <button onclick="switchTab('coupons')" id="nav-coupons" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-percent w-4 text-center text-slate-400"></i><span>الخصومات</span>
            </button>
            <button onclick="switchTab('deviceradar')" id="nav-deviceradar" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-mobile-screen-button w-4 text-center text-slate-400"></i><span>الأجهزة المتصلة</span>
            </button>
            <button onclick="switchTab('audit')" id="nav-audit" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-clock-rotate-left w-4 text-center text-slate-400"></i><span>سجل التدقيق</span>
            </button>
            <button onclick="switchTab('bi')" id="nav-bi" class="nav-tab-btn w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl text-xs font-bold text-slate-600 hover:bg-slate-50 transition">
              <i class="fa-solid fa-chart-line w-4 text-center text-slate-400"></i><span>التقارير</span>
            </button>
          </div>
        </nav>

        <!-- System Pill Footer -->
        <div class="p-3 bg-slate-50 rounded-2xl border border-slate-100 flex items-center justify-between text-[11px] font-bold text-slate-500">
          <div class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
            <span>السيرفر متصل</span>
          </div>
          <span class="text-[10px] font-mono font-black text-slate-400">PG + Go</span>
        </div>
      </aside>
`
}

func GetMobileDrawer() string {
	return `      <!-- MOBILE SIDEBAR BACKDROP & DRAWER -->
      <div id="mobile-sidebar-backdrop" onclick="toggleMobileSidebar()" class="fixed inset-0 z-40 bg-slate-900/60 backdrop-blur-sm hidden lg:hidden"></div>
      <aside id="mobile-sidebar" class="fixed inset-y-0 right-0 z-50 w-72 bg-white p-5 flex flex-col space-y-4 shadow-2xl transform translate-x-full transition-transform duration-300 lg:hidden">
        <div class="flex items-center justify-between pb-2 border-b border-slate-100">
          <div class="flex items-center gap-2">
            <div class="w-9 h-9 rounded-xl overflow-hidden bg-brand-light flex items-center justify-center font-black flex-shrink-0">
              <img src="/assets/appicon.png" class="w-full h-full object-contain" alt="فينوس">
            </div>
            <span class="font-black text-slate-900 text-sm">فينوس</span>
          </div>
          <button onclick="toggleMobileSidebar()" class="p-2 text-slate-400 hover:text-slate-600"><i class="fa-solid fa-xmark text-lg"></i></button>
        </div>
        <nav class="flex-1 space-y-1 overflow-y-auto pr-1 text-xs font-bold">
          <button onclick="switchTab('stats'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-chart-pie w-4 text-slate-400"></i><span>الرئيسية</span>
          </button>
          <button onclick="switchTab('users'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-users w-4 text-slate-400"></i><span>المشتركين</span>
          </button>
          <button onclick="switchTab('activation'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-key w-4 text-slate-400"></i><span>الباقات</span>
          </button>
          <button onclick="switchTab('wallet'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-wallet w-4 text-slate-400"></i><span>المحافظ</span>
          </button>
          <button onclick="switchTab('cards'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-sim-card w-4 text-slate-400"></i><span>البطاقات</span>
          </button>
          <button onclick="switchTab('ledger'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-book-journal-whills w-4 text-slate-400"></i><span>السجل المالي</span>
          </button>
          <button onclick="switchTab('fraud'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-shield-halved w-4 text-slate-400"></i><span>الحماية</span>
          </button>
          <button onclick="switchTab('reconcile'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-scale-balanced w-4 text-slate-400"></i><span>التسويات</span>
          </button>
          <button onclick="switchTab('debts'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-hand-holding-dollar w-4 text-slate-400"></i><span>الائتمان</span>
          </button>
          <button onclick="switchTab('close'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-file-invoice-dollar w-4 text-slate-400"></i><span>كشوف الحساب</span>
          </button>
          <button onclick="switchTab('pricing'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-tags w-4 text-slate-400"></i><span>الأسعار</span>
          </button>
          <button onclick="switchTab('radar'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-tower-broadcast w-4 text-slate-400"></i><span>مؤشر النفاد</span>
          </button>
          <button onclick="switchTab('coupons'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-percent w-4 text-slate-400"></i><span>الخصومات</span>
          </button>
          <button onclick="switchTab('appconfig'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-sliders w-4 text-slate-400"></i><span>إعدادات المنصة</span>
          </button>
          <button onclick="switchTab('deviceradar'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-mobile-screen-button w-4 text-slate-400"></i><span>الأجهزة المتصلة</span>
          </button>
          <button onclick="switchTab('security'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-shield w-4 text-slate-400"></i><span>الرقابة والأمان</span>
          </button>
          <button onclick="switchTab('bi'); toggleMobileSidebar()" class="w-full text-right px-3.5 py-2.5 rounded-xl hover:bg-slate-50 text-slate-700 flex items-center gap-3">
            <i class="fa-solid fa-chart-line w-4 text-slate-400"></i><span>التقارير</span>
          </button>
        </nav>
      </aside>
`
}

func GetTopHeader() string {
	return `      <!-- MAIN CONTENT WRAPPER -->
      <div class="flex-1 flex flex-col min-w-0 overflow-hidden">
        <!-- TOP NAVBAR -->
        <header class="bg-white border-b border-slate-200/80 px-4 sm:px-6 py-3 flex items-center justify-between gap-4 z-10 flex-shrink-0">
          <div class="flex items-center gap-3">
            <button onclick="toggleMobileSidebar()" class="lg:hidden p-2 rounded-xl bg-slate-50 hover:bg-slate-100 text-slate-600"><i class="fa-solid fa-bars text-sm"></i></button>
            <img src="/assets/appicon.png" class="w-7 h-7 rounded-lg object-contain lg:hidden shadow-sm" alt="Logo">
            <div class="flex items-center gap-2">
              <span class="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse"></span>
              <span id="current-tab-title" class="text-sm font-black text-slate-800">الرئيسية</span>
            </div>
          </div>

          <!-- Top Navigation Right Info -->
          <div class="flex items-center gap-3 sm:gap-4">
            <!-- Baghdad Clock -->
            <div class="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-2xl bg-slate-50 border border-slate-100 text-xs font-bold text-slate-600">
              <i class="fa-regular fa-clock text-brand text-xs"></i>
              <span id="baghdad-clock">—:—:— بتوقيت بغداد</span>
            </div>

            <!-- Admin Profile & Logout -->
            <div class="flex items-center gap-2">
              <div class="flex items-center gap-2 pl-2 sm:pl-3 border-l border-slate-100">
                <div class="w-8 h-8 rounded-full bg-brand-light text-brand flex items-center justify-center font-black text-xs">
                  <i class="fa-solid fa-user-shield"></i>
                </div>
                <div class="hidden md:block text-right">
                  <span id="admin-name" class="block text-xs font-bold text-slate-800 leading-tight">المدير العام</span>
                  <span class="block text-[10px] text-emerald-600 font-bold leading-tight">SuperAdmin</span>
                </div>
              </div>
              <button onclick="handleLogout()" title="تسجيل الخروج" class="p-2 text-slate-400 hover:text-rose-600 rounded-xl hover:bg-rose-50 transition">
                <i class="fa-solid fa-arrow-right-from-bracket text-sm"></i>
              </button>
            </div>
          </div>
        </header>
`
}
