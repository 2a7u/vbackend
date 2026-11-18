package main

func GetTabsEnterprise() string {
	return `        <!-- ========================================================================= -->
        <!-- TAB 14: PROMOTIONS & COUPONS (Phase 4)                                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-coupons" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الخصومات</h2>
              <p class="text-xs text-slate-500 mt-0.5">إدارة أكواد الخصم، العمولات التلقائية للمناديب، ونسب الخصم</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openAddCouponModal()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-plus"></i>
                <span>إنشاء كود خصم جديد</span>
              </button>
            </div>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">كود الخصم</th>
                    <th class="py-3 px-3">نوع وقيمة الخصم</th>
                    <th class="py-3 px-3">الاستخدامات / الحد الأقصى</th>
                    <th class="py-3 px-3">المندوب المرتبط</th>
                    <th class="py-3 px-3">عمولة المندوب (د.ع)</th>
                    <th class="py-3 px-3">الحالة والصلاحية</th>
                  </tr>
                </thead>
                <tbody id="coupons-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="6" class="py-8 text-center text-slate-400">جاري تحميل الخصومات...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 15: REMOTE APP CONFIG (Phase 5)                                       -->
        <!-- ========================================================================= -->
        <section id="tab-content-appconfig" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">إعدادات المنصة</h2>
              <p class="text-xs text-slate-500 mt-0.5">الصيانة، إجبار التحديث، Feature Flags، البنرات وشريط الأخبار</p>
            </div>
          </div>

          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <!-- Maintenance Mode & Kill-Switch -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-8 h-8 rounded-xl bg-amber-50 text-amber-600 flex items-center justify-center text-sm font-black"><i class="fa-solid fa-power-off"></i></div>
                  <div>
                    <h3 class="text-sm font-black text-slate-800">الصيانة</h3>
                    <p class="text-[11px] text-slate-400">تعطيل وصول المشتركين فورياً مع استثناء المدراء</p>
                  </div>
                </div>
                <input type="checkbox" id="cfg-maintenance-toggle" class="w-5 h-5 accent-brand cursor-pointer">
              </div>
              <div>
                <label class="text-[11px] font-bold text-slate-500 block mb-1">رسالة الصيانة المعروضة بالتطبيق:</label>
                <input type="text" id="cfg-maintenance-msg" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
              </div>
              <button onclick="saveMaintenanceConfig()" class="w-full py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ الصيانة</button>
            </div>

            <!-- Force Update -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-8 h-8 rounded-xl bg-blue-50 text-brand flex items-center justify-center text-sm font-black"><i class="fa-solid fa-cloud-arrow-up"></i></div>
                  <div>
                    <h3 class="text-sm font-black text-slate-800">إجبار التحديث (Force Update)</h3>
                    <p class="text-[11px] text-slate-400">منع النسخ القديمة من الاتصال بالسيرفر</p>
                  </div>
                </div>
                <input type="checkbox" id="cfg-force-update-toggle" class="w-5 h-5 accent-brand cursor-pointer">
              </div>
              <div class="grid grid-cols-2 gap-2">
                <div>
                  <label class="text-[11px] font-bold text-slate-500 block mb-1">الحد الأدنى للإصدار:</label>
                  <input type="text" id="cfg-min-version" placeholder="1.0.0" class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
                </div>
                <div>
                  <label class="text-[11px] font-bold text-slate-500 block mb-1">رابط التحميل المباشر:</label>
                  <input type="text" id="cfg-update-url" placeholder="https://..." class="w-full px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-medium outline-none focus:border-brand">
                </div>
              </div>
              <button onclick="saveForceUpdateConfig()" class="w-full py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ إعدادات التحديث</button>
            </div>

            <!-- Feature Flags -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4 md:col-span-2">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-toggle-on text-brand"></i>
                <span>الخدمات</span>
              </h3>
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700">
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-zaincash" class="accent-brand w-4 h-4"><span>بوابة زين كاش</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-vouchers" class="accent-brand w-4 h-4"><span>بيع البطاقات</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-trial" class="accent-brand w-4 h-4"><span>التجربة المجانية</span>
                </label>
                <label class="flex items-center gap-2 p-3 bg-slate-50 rounded-xl border border-slate-200 cursor-pointer">
                  <input type="checkbox" id="ff-debts" class="accent-brand w-4 h-4"><span>الديون والآجل</span>
                </label>
              </div>
              <button onclick="saveFeatureFlagsConfig()" class="px-5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm transition">حفظ الـ Feature Flags</button>
            </div>

            <!-- Safe Factory Data Reset (Zero-State Production Wipe) -->
            <div class="bg-rose-50/70 p-5 rounded-3xl border border-rose-200 shadow-sm space-y-3 md:col-span-2">
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                <div class="flex items-start gap-3">
                  <div class="w-10 h-10 rounded-2xl bg-rose-100 text-rose-600 flex items-center justify-center text-base font-black flex-shrink-0">
                    <i class="fa-solid fa-triangle-exclamation"></i>
                  </div>
                  <div>
                    <h3 class="text-sm font-black text-rose-800">إعادة ضبط المصنع وتصفير البيانات (Factory Reset)</h3>
                    <p class="text-xs text-rose-600/90 mt-0.5">حذف جميع المشتركين التجريبيين، السجلات المالية، البطاقات، والأكواد للبدء من الصفر تماماً مع الحفاظ على حساب المدير العام والباقات الافتراضية.</p>
                  </div>
                </div>
                <button onclick="triggerFactoryReset()" class="px-5 py-2.5 bg-rose-600 hover:bg-rose-700 text-white rounded-2xl font-black text-xs shadow-sm transition flex items-center justify-center gap-2 flex-shrink-0">
                  <i class="fa-solid fa-rotate-left"></i>
                  <span>تصفير المنصة إلى الصفر</span>
                </button>
              </div>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 16: LIVE DEVICE RADAR & COMMANDS (Phase 6)                            -->
        <!-- ========================================================================= -->
        <section id="tab-content-deviceradar" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الأجهزة المتصلة</h2>
              <p class="text-xs text-slate-500 mt-0.5">مراقبة اتصال الأجهزة لحظياً، سرعة الاستجابة، وإرسال أوامر القائمة البيضاء المعتمدة</p>
            </div>
            <button onclick="loadDeviceRadarData()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-2 transition">
              <i class="fa-solid fa-rotate-right"></i>
              <span>تحديث الرادار الحي</span>
            </button>
          </div>

          <div class="bg-white rounded-3xl border border-slate-200/80 shadow-sm overflow-hidden p-4 space-y-3">
            <h3 class="text-sm font-black text-slate-800">الأجهزة المتصلة أونلاين الآن عبر WebSocket</h3>
            <div class="overflow-x-auto">
              <table class="w-full text-right border-collapse text-xs">
                <thead>
                  <tr class="bg-slate-50/80 border-b border-slate-200 text-slate-500 font-bold">
                    <th class="py-3 px-3">المشترك / المتجر</th>
                    <th class="py-3 px-3">موديل الهاتف</th>
                    <th class="py-3 px-3">إصدار التطبيق</th>
                    <th class="py-3 px-3">زمن الاستجابة (Latency)</th>
                    <th class="py-3 px-3">الشاشة الحالية</th>
                    <th class="py-3 px-3">آخر نشاط</th>
                    <th class="py-3 px-3 text-center">أوامر عن بعد</th>
                  </tr>
                </thead>
                <tbody id="online-devices-table-body" class="divide-y divide-slate-100 font-medium">
                  <tr><td colspan="7" class="py-8 text-center text-slate-400">جاري مسح الأجهزة المتصلة...</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 17: SECURITY, WAF & API KEYS (Phase 7)                                -->
        <!-- ========================================================================= -->
        <section id="tab-content-security" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">الرقابة والأمان</h2>
              <p class="text-xs text-slate-500 mt-0.5">قوائم حظر العناوين IP، كشف محاولات الاختراق، وتوليد مفاتيح API المشفرة</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="openAddIPRuleModal()" class="px-3.5 py-2 bg-rose-50 hover:bg-rose-100 text-rose-700 border border-rose-200 rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-ban"></i><span>إضافة قاعدة IP</span>
              </button>
              <button onclick="openAddAPIKeyModal()" class="px-3.5 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                <i class="fa-solid fa-key"></i><span>توليد مفتاح API جديد</span>
              </button>
            </div>
          </div>

          <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <!-- IP Rules Table -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-shield-halved text-rose-600"></i>
                <span>قواعد جدار الحماية (IP Rules & Bans)</span>
              </h3>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">عنوان IP</th>
                      <th class="py-2.5 px-3">النوع</th>
                      <th class="py-2.5 px-3">السبب</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="ip-rules-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="4" class="py-6 text-center text-slate-400">جاري تحميل قواعد الحظر...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- API Keys Table -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-key text-brand"></i>
                <span>مفاتيح API الخارجية المعتمدة</span>
              </h3>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">اسم المفتاح</th>
                      <th class="py-2.5 px-3">البادئة</th>
                      <th class="py-2.5 px-3">الدور</th>
                      <th class="py-2.5 px-3">الحالة</th>
                    </tr>
                  </thead>
                  <tbody id="api-keys-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="4" class="py-6 text-center text-slate-400">جاري تحميل مفاتيح API...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Limits Blocks Table (Stage 1) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                    <i class="fa-solid fa-user-slash text-rose-600"></i>
                    <span>قائمة الحظر والحدود (Limits & Anti-Brute-Force)</span>
                  </h3>
                  <p class="text-[11px] text-slate-400">المشتركون وعناوين IP المحظورة مؤقتاً لتجاوز محاولات التفعيل أو معدل الطلبات</p>
                </div>
                <button onclick="loadLimitsBlocks()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition flex items-center gap-1.5">
                  <i class="fa-solid fa-rotate-right"></i><span>تحديث</span>
                </button>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">النوع</th>
                      <th class="py-2.5 px-3">المعرف / المفتاح</th>
                      <th class="py-2.5 px-3">السبب</th>
                      <th class="py-2.5 px-3">المحاولات</th>
                      <th class="py-2.5 px-3">ينتهي الحظر في</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="limits-blocks-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="6" class="py-6 text-center text-slate-400">جاري تحميل المحظورين...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Dead Jobs Queue (Stage 2) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                    <i class="fa-solid fa-triangle-exclamation text-amber-500"></i>
                    <span>المهام الخلفية المتعثرة (Dead Jobs Queue)</span>
                  </h3>
                  <p class="text-[11px] text-slate-400">المهام التي استنفدت 10 محاولات تنفيذ متتالية (مثل فحص زين كاش أو إشعارات الشبكة) وتحتاج تدخلاً أو إعادة محاولة</p>
                </div>
                <button onclick="loadDeadJobs()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition flex items-center gap-1.5">
                  <i class="fa-solid fa-rotate-right"></i><span>تحديث</span>
                </button>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">رقم المهمة</th>
                      <th class="py-2.5 px-3">المشترك</th>
                      <th class="py-2.5 px-3">نوع المهمة</th>
                      <th class="py-2.5 px-3">المحاولات</th>
                      <th class="py-2.5 px-3">الخطأ الأخير</th>
                      <th class="py-2.5 px-3">تاريخ الإنشاء</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="dead-jobs-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="7" class="py-6 text-center text-slate-400">جاري تحميل المهام المتعثرة...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Notify Channels & Fallback Configuration (Stage 3) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                    <i class="fa-solid fa-bullhorn text-brand"></i>
                    <span>قنوات الإشعارات والـ Fallback الموحد (Notify Channels)</span>
                  </h3>
                  <p class="text-[11px] text-slate-400">إدارة قنوات الإشعار (Telegram, WhatsApp, SMS, Push) وترتيب الأولوية التلقائي عند تعطل إحداها</p>
                </div>
                <div class="flex items-center gap-2">
                  <button onclick="openAddNotifyChannelModal()" class="px-3.5 py-1.5 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                    <i class="fa-solid fa-plus"></i><span>إضافة قناة</span>
                  </button>
                  <button onclick="loadNotifyChannels()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition flex items-center gap-1.5">
                    <i class="fa-solid fa-rotate-right"></i><span>تحديث</span>
                  </button>
                </div>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">الأولوية (Fallback Order)</th>
                      <th class="py-2.5 px-3">نوع القناة</th>
                      <th class="py-2.5 px-3">الحالة</th>
                      <th class="py-2.5 px-3">الإعدادات</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="notify-channels-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="5" class="py-6 text-center text-slate-400">جاري تحميل قنوات الإشعار...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Seal Receipt Verifier Tool (Stage 3) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                <i class="fa-solid fa-stamp text-emerald-600"></i>
                <span>فاحص الأختام الرقمية وصحة الفواتير (Seal Verifier)</span>
              </h3>
              <p class="text-[11px] text-slate-400">تحقق فوري من أي توقيع أو رمز فاتورة HMAC-SHA256 لمعرفة المتجر والمبلغ وتوقيت بغداد المعتمد</p>
              <div class="flex gap-2">
                <input type="text" id="seal-verify-token-input" placeholder="ادخل رمز الختم (مثال: v-ab12cd34ef)" class="flex-1 px-3 py-2 bg-slate-50 border border-slate-200 rounded-xl text-xs font-mono font-bold text-slate-800 outline-none focus:border-brand">
                <button onclick="verifySealTokenManually()" class="px-5 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl text-xs font-bold shadow-sm transition flex items-center gap-1.5">
                  <i class="fa-solid fa-check-double"></i><span>تحقق الآن</span>
                </button>
              </div>
              <div id="seal-verify-result-box" class="p-4 bg-slate-50 rounded-2xl border border-slate-200 hidden text-xs space-y-1"></div>
            </div>

            <!-- Risk Engine & Profiles (Stage 4) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                    <i class="fa-solid fa-triangle-exclamation text-rose-600"></i>
                    <span>محرك تقييم المخاطر والاحتيال (Risk Profiles)</span>
                  </h3>
                  <p class="text-[11px] text-slate-400">تقييم نقاط الخطر تلقائياً بناءً على العمليات الليلية، تسارع الطلبات، ومحاولات تسجيل الدخول الفاشلة</p>
                </div>
                <button onclick="loadRiskProfiles()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition flex items-center gap-1.5">
                  <i class="fa-solid fa-rotate-right"></i><span>تحديث</span>
                </button>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">رقم التاجر / المشترك</th>
                      <th class="py-2.5 px-3">مؤشر الخطر (0-100)</th>
                      <th class="py-2.5 px-3">المؤشرات المرصودة (Flags)</th>
                      <th class="py-2.5 px-3">حالة الحساب</th>
                      <th class="py-2.5 px-3">آخر تقييم</th>
                      <th class="py-2.5 px-3 text-center">إجراءات</th>
                    </tr>
                  </thead>
                  <tbody id="risk-profiles-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="6" class="py-6 text-center text-slate-400">جاري تحميل تقييمات المخاطر...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- B2B API Keys v2 (Stage 4) -->
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3 lg:col-span-2">
              <div class="flex items-center justify-between">
                <div>
                  <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                    <i class="fa-solid fa-network-wired text-brand"></i>
                    <span>مفاتيح الربط البرمجي B2B (API Keys v2)</span>
                  </h3>
                  <p class="text-[11px] text-slate-400">مفاتيح API مشفرة بـ SHA-256 للشركاء مع تحديد الصلاحيات وسقوف الاستهلاك ووثائق OpenAPI</p>
                </div>
                <div class="flex items-center gap-2">
                  <a href="/api/docs/openapi.json" target="_blank" class="px-3 py-1.5 bg-slate-100 hover:bg-slate-200 text-slate-700 rounded-xl font-bold text-xs transition flex items-center gap-1">
                    <i class="fa-solid fa-book-open"></i><span>وثائق OpenAPI</span>
                  </a>
                  <button onclick="openAddApiKeyV2Modal()" class="px-3.5 py-1.5 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-sm flex items-center gap-1.5 transition">
                    <i class="fa-solid fa-plus"></i><span>توليد مفتاح جديد</span>
                  </button>
                  <button onclick="loadApiKeysV2()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition">
                    <i class="fa-solid fa-rotate-right"></i>
                  </button>
                </div>
              </div>
              <div class="overflow-x-auto">
                <table class="w-full text-right border-collapse text-xs">
                  <thead>
                    <tr class="bg-slate-50 border-b border-slate-200 text-slate-500 font-bold">
                      <th class="py-2.5 px-3">التسمية</th>
                      <th class="py-2.5 px-3">البادئة (Prefix)</th>
                      <th class="py-2.5 px-3">الصلاحيات (Scopes)</th>
                      <th class="py-2.5 px-3">الحد / دقيقة</th>
                      <th class="py-2.5 px-3">مرات الاستخدام</th>
                      <th class="py-2.5 px-3">الحالة</th>
                      <th class="py-2.5 px-3 text-center">إجراء</th>
                    </tr>
                  </thead>
                  <tbody id="api-keys-v2-table-body" class="divide-y divide-slate-100 font-medium">
                    <tr><td colspan="7" class="py-6 text-center text-slate-400">جاري تحميل مفاتيح API v2...</td></tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </section>

        <!-- ========================================================================= -->
        <!-- TAB 18: BUSINESS INTELLIGENCE & CLOUD BACKUP (Phase 8)                    -->
        <!-- ========================================================================= -->
        <section id="tab-content-bi" class="tab-pane hidden space-y-6">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h2 class="text-xl font-black text-slate-800">التقارير</h2>
              <p class="text-xs text-slate-500 mt-0.5">تحليلات الأرباح الصافية، التقرير اليومي التلقائي (00:00 بغداد)، والنسخ السريع للبيانات</p>
            </div>
            <div class="flex items-center gap-2">
              <button onclick="sendReportsNow()" class="px-4 py-2 bg-brand hover:bg-brand-dark text-white rounded-xl font-bold text-xs shadow-md shadow-brand/20 flex items-center gap-2 transition active:scale-[0.99]">
                <i class="fa-solid fa-paper-plane"></i>
                <span>أرسل التقرير الآن</span>
              </button>
              <button onclick="downloadSecureBackup()" class="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-xl font-bold text-xs shadow-md shadow-emerald-600/20 flex items-center gap-2 transition">
                <i class="fa-solid fa-download"></i>
                <span>نسخة احتياطية (JSON)</span>
              </button>
            </div>
          </div>

          <!-- BI KPI Cards -->
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">صافي أرباح البطاقات</span>
              <div class="text-xl font-black text-emerald-600" id="bi-vouchers-profit">0 د.ع</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">إيراد الاشتراكات الإلكترونية</span>
              <div class="text-xl font-black text-brand" id="bi-sub-profit">0 د.ع</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">معدل الاحتفاظ (Retention)</span>
              <div class="text-xl font-black text-purple-600" id="bi-retention-rate">0%</div>
            </div>
            <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-1">
              <span class="text-slate-400 text-xs font-bold">معدل الانسحاب (Churn)</span>
              <div class="text-xl font-black text-rose-600" id="bi-churn-rate">0%</div>
            </div>
          </div>

          <!-- Live Server Health Box -->
          <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-3">
            <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
              <i class="fa-solid fa-server text-emerald-600"></i>
              <span>مؤشرات أداء السيرفر المباشرة (Live Telemetry)</span>
            </h3>
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700">
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">زمن استجابة DB</span>
                <span id="bi-db-latency" class="text-sm font-black text-emerald-600">0 ms</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">الـ Goroutines النشطة</span>
                <span id="bi-goroutines" class="text-sm font-black text-brand">0</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">استهلاك RAM المستخدم</span>
                <span id="bi-ram" class="text-sm font-black text-slate-800">0 MB</span>
              </div>
              <div class="p-3 bg-slate-50 rounded-xl border border-slate-200/80">
                <span class="text-slate-400 block text-[10px]">توقيت السيرفر الرسمي</span>
                <span id="bi-server-time" class="text-xs font-bold text-slate-600">—</span>
              </div>
            </div>
          </div>

          <!-- 7-Day Liquidity Forecast (Stage 4) -->
          <div class="bg-white p-5 rounded-3xl border border-slate-200/80 shadow-sm space-y-4">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="text-sm font-black text-slate-800 flex items-center gap-2">
                  <i class="fa-solid fa-chart-line text-brand"></i>
                  <span>توقعات السيولة المالية للـ 7 أيام القادمة (Liquidity Forecast)</span>
                </h3>
                <p class="text-[11px] text-slate-400">حساب المتوسط المتحرك لوتيرة المبيعات والتنبؤ بالسيولة وأيام الأمان المالي</p>
              </div>
              <button onclick="loadForecastData()" class="px-3 py-1.5 bg-slate-50 hover:bg-slate-100 text-slate-700 border border-slate-200 rounded-xl font-bold text-xs transition flex items-center gap-1.5">
                <i class="fa-solid fa-rotate-right"></i><span>تحديث التوقعات</span>
              </button>
            </div>

            <!-- Forecast KPI Summary Cards -->
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-4 text-xs font-bold text-slate-700">
              <div class="p-4 bg-emerald-50/60 rounded-2xl border border-emerald-200/60">
                <span class="text-emerald-700 block text-[11px] font-bold">الرصيد الفعلي الحالي</span>
                <span id="forecast-current-balance" class="text-base font-black text-emerald-800 block mt-1">0 د.ع</span>
              </div>
              <div class="p-4 bg-blue-50/60 rounded-2xl border border-blue-200/60">
                <span class="text-blue-700 block text-[11px] font-bold">التدفق المتوقع للأسبوع</span>
                <span id="forecast-projected-weekly" class="text-base font-black text-brand block mt-1">0 د.ع</span>
              </div>
              <div class="p-4 bg-purple-50/60 rounded-2xl border border-purple-200/60">
                <span class="text-purple-700 block text-[11px] font-bold">أيام الأمان المالي (Runway)</span>
                <span id="forecast-runway-days" class="text-base font-black text-purple-800 block mt-1">0 يوم</span>
              </div>
              <div class="p-4 bg-amber-50/60 rounded-2xl border border-amber-200/60">
                <span class="text-amber-700 block text-[11px] font-bold">معدل الاستنزاف اليومي</span>
                <span id="forecast-burn-rate" class="text-base font-black text-amber-800 block mt-1">0 د.ع / يوم</span>
              </div>
            </div>

            <!-- 7-Day Projected Cards Grid -->
            <div>
              <span class="text-xs font-black text-slate-700 block mb-2">التوزيع التقديري للمبيعات والسيولة لكل يوم:</span>
              <div class="grid grid-cols-1 sm:grid-cols-7 gap-3" id="forecast-daily-grid">
                <div class="col-span-full py-4 text-center text-slate-400 font-bold text-xs">جاري حساب التوقعات الرياضية...</div>
              </div>
            </div>
          </div>
        </section>
`
}
