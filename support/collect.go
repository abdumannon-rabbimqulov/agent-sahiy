// Yig'ish rejimi: javobsiz suhbatlardagi buyurtma raqamlarini (matn va
// rasm) tekshirib, status va muammo turini bazaga yozadi.
//
// Bu yo'l mijozga HECH NARSA yubormaydi: RunChain, deliverChat, o'qilgan
// belgisi, suhbatni yopish, Telegram, order_issues va markHandled
// chaqirilmaydi. Suhbat javobsiz holatda qoladi.
package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrCollectMode - yig'ish rejimida AI mijozga javob yozmaydi.
var ErrCollectMode = errors.New("yig'ish rejimi yoqiq: AI mijozga javob yubormaydi")

// tokenFileMu - bir nechta worker token faylini bir vaqtda yozmasin.
var tokenFileMu sync.Mutex

// blockedByCollectMode - bu javob yig'ish rejimida to'sib qo'yiladimi.
// Xodimning Telegram javobi o'tadi: u AI emas, odam yozgan javob.
func blockedByCollectMode(in *Interaction) bool {
	return CollectMode() && in.Source != SourceTelegram
}

const (
	collectPageSize        = 100
	collectMaxPages        = DefaultUnansweredMaxPages
	collectMaxAttempts     = 3
	collectMaxNumbers      = 10
	collectIdleInterval    = 5 * time.Minute
	collectTick            = 15 * time.Second
	collectStaleAfter      = 24 * time.Hour
	collectStaleBatch      = 200
	collectChatTimeout     = 3 * time.Minute
	collectAccountPageSize = 100
	collectBucketEvery     = time.Hour
	collectBackoffMax      = 8.0
	collectBackoffEvery    = 20
)

// CollectStatus - hozirgi yurish holati (panel uchun).
type CollectStatus struct {
	Mode      bool       `json:"mode"`
	Wanted    bool       `json:"wanted"`
	Running   bool       `json:"running"`
	RunID     uint       `json:"run_id"`
	Total     int        `json:"total"`
	Done      int        `json:"done"`
	Errors    int        `json:"errors"`
	Remaining int        `json:"remaining"`
	PerMin    float64    `json:"per_min"`
	ETASec    int        `json:"eta_sec"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	GapMS     int64      `json:"gap_ms"`
	Backoff   float64    `json:"backoff"`
	Workers   int        `json:"workers"`
	TargetMin int        `json:"target_min"`
	Note      string     `json:"note,omitempty"`
}

// collector - bitta jarayondagi yagona yurish holati.
var collector = struct {
	sync.Mutex
	st      CollectStatus
	okRun   int
	active  int
	fatal   error
	nextRun time.Time
}{}

func collectWanted() bool {
	if !CollectMode() {
		return false
	}
	return GetBoolSetting(SettingCollectRun, false) || (AgentEnabled() && PollEnabled())
}

// StartCollector - yig'uvchi siklini ishga tushiradi. Agent yoqilganda
// yoki paneldan yurish so'ralganda ishlaydi.
func StartCollector(ctx context.Context) {
	go func() {
		for {
			if err := refreshPurchasedBuckets(); err != nil {
				log.Printf("yig'ish: status 3 yoshi: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(collectBucketEvery):
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(collectTick):
			}
			if !collectWanted() {
				continue
			}
			manual := GetBoolSetting(SettingCollectRun, false)
			collector.Lock()
			wait := !manual && time.Now().Before(collector.nextRun)
			collector.Unlock()
			if wait {
				continue
			}
			max := 0
			if manual {
				max = GetIntSetting(SettingCollectMax, 0, 0, 1000000)
			}
			if err := runCollect(ctx, max); err != nil {
				log.Printf("yig'ish: %v", err)
			}
			collector.Lock()
			collector.nextRun = time.Now().Add(collectIdleInterval)
			collector.Unlock()
			if manual && DB != nil {
				// Bir martalik yurish tugadi.
				if err := SetSetting(DB, SettingCollectRun, "false"); err != nil {
					log.Printf("yig'ish: sozlama: %v", err)
				}
			}
		}
	}()
}

// refreshPurchasedBuckets - status 3 buyurtmasi yoshi oshganda muammo turiga
// o'tkazadi yoki guruhini yangilaydi. Faqat baza: tashqi so'rov yo'q.
// Chegaralar ClassifyOrder bilan bir xil: to'liq kun 5, 20, 30 dan oshgan.
func refreshPurchasedBuckets() error {
	if DB == nil {
		return nil
	}
	return DB.Exec(`INSERT INTO collect_findings (order_sn, type, bucket, user_id, anchor_at, anchor_kind)
		SELECT order_sn, ?, CASE
			WHEN order_created_at <= now() - interval '31 days' THEN '30'
			WHEN order_created_at <= now() - interval '21 days' THEN '20'
			ELSE '5' END,
			user_id, order_created_at, ?
		FROM collect_orders
		WHERE status = ? AND pay_status = 1 AND verified
			AND order_created_at <= now() - interval '6 days'
		ON CONFLICT (order_sn, type) DO UPDATE SET bucket = EXCLUDED.bucket`,
		FindPurchased, AnchorCreated, StatusPaid).Error
}

// CollectStatusNow - panel uchun holat.
func CollectStatusNow() CollectStatus {
	collector.Lock()
	st := collector.st
	collector.Unlock()
	st.Mode, st.Wanted = CollectMode(), collectWanted()
	st.Workers, st.TargetMin = CollectWorkers(), CollectTargetMin()
	st.Remaining = st.Total - st.Done
	if st.Remaining < 0 {
		st.Remaining = 0
	}
	if st.Running && st.StartedAt != nil && st.Done > 0 {
		mins := time.Since(*st.StartedAt).Minutes()
		if mins > 0 {
			st.PerMin = float64(st.Done) / mins
			st.ETASec = int(float64(st.Remaining) / st.PerMin * 60)
		}
	}
	return st
}

// collectFetchQueue - javobsiz va hali o'rganilmagan suhbatlar.
func collectFetchQueue() ([]Chat, error) {
	chats, err := withToken(func(baseURL, token string) ([]Chat, error) {
		return FetchUnansweredChats(baseURL, token, collectMaxPages, collectPageSize)
	})
	if err != nil {
		return nil, err
	}
	var rows []CollectChat
	if err := DB.Select("conversation_id, last_message_at, state, attempts").Find(&rows).Error; err != nil {
		return nil, err
	}
	seen := make(map[int64]CollectChat, len(rows))
	for _, r := range rows {
		seen[r.ConversationID] = r
	}
	todo := collectPending(chats, seen)
	// Javobsiz ro'yxatda qolgan, ilgari o'rganilgan suhbatlar: yig'uvchi
	// suhbat holatiga tegmaganining isboti.
	still := 0
	for _, c := range chats {
		if _, ok := seen[c.ID]; ok {
			still++
		}
	}
	log.Printf("yig'ish: navbat: %d javobsiz, %d ishlanadi, %d tasi avval o'rganilgan va hali javobsiz",
		len(chats), len(todo), still)
	return todo, nil
}

// collectPending - qaysi suhbat ishlanishi kerak, eng uzoq kutgani birinchi.
// Yangi xabar kelmagan bo'lsa o'rganilgan suhbat qayta ko'rilmaydi; xato
// bilan tugagani collectMaxAttempts martagacha qayta uriniladi.
func collectPending(chats []Chat, seen map[int64]CollectChat) []Chat {
	return pendingSorted(chats, func(c Chat) bool {
		r, ok := seen[c.ID]
		if !ok || r.LastMessageAt != c.MsCreatedAt {
			return true
		}
		retry := r.State == CollectStatePartial || r.State == CollectStateError
		return retry && r.Attempts < collectMaxAttempts
	})
}

func pendingSorted(chats []Chat, keep func(Chat) bool) []Chat {
	out := make([]Chat, 0, len(chats))
	for _, c := range chats {
		if c.Unanswered() && keep(c) {
			out = append(out, c)
		}
	}
	sortChatsOldest(out)
	return out
}

// runCollect - bitta yurish. Natija har suhbatdan keyin darhol bazaga
// yoziladi, shuning uchun jarayon to'xtasa keyingi yurish davom ettiradi.
func runCollect(ctx context.Context, max int) error {
	if DB == nil {
		return errors.New("baza ulanmagan")
	}
	todo, err := collectFetchQueue()
	if err != nil {
		return fmt.Errorf("navbat: %w", err)
	}
	if max > 0 && len(todo) > max {
		todo = todo[:max]
	}
	stale := []CollectOrder{}
	if max == 0 {
		DB.Where("verified = ? AND received = ? AND checked_at < ?", true, false, time.Now().Add(-collectStaleAfter)).
			Order("checked_at asc").Limit(collectStaleBatch).Find(&stale)
	}
	total := len(todo) + len(stale)
	if total == 0 {
		return nil
	}

	now := time.Now()
	run := CollectRun{StartedAt: now, Total: total, Max: max}
	if err := DB.Create(&run).Error; err != nil {
		return err
	}
	collector.Lock()
	collector.st = CollectStatus{Running: true, RunID: run.ID, Total: total, StartedAt: &now, Backoff: 1}
	collector.okRun, collector.fatal = 0, nil
	collector.Unlock()
	log.Printf("yig'ish: yurish #%d boshlandi: %d suhbat, %d qayta tekshiruv", run.ID, len(todo), len(stale))

	env := newCollectEnv()
	deadline := now.Add(time.Duration(CollectTargetMin()) * time.Minute)
	var wg sync.WaitGroup

	dispatch := func(remaining int, job func(context.Context) error) bool {
		if !collectWait(ctx, deadline, remaining) {
			return false
		}
		collector.Lock()
		collector.active++
		collector.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			jobCtx, cancel := context.WithTimeout(ctx, collectChatTimeout)
			defer cancel()
			err := job(jobCtx)
			collectAfterJob(err)
			collector.Lock()
			collector.active--
			done, errs := collector.st.Done, collector.st.Errors
			collector.Unlock()
			if done%10 == 0 {
				DB.Model(&CollectRun{}).Where("id = ?", run.ID).Updates(map[string]any{"done": done, "errors": errs})
			}
		}()
		return true
	}

	stopped := false
	for i := range todo {
		c := todo[i]
		if !dispatch(total-i, func(jc context.Context) error { return env.processChat(jc, c) }) {
			stopped = true
			break
		}
	}
	if !stopped {
		for i := range stale {
			o := stale[i]
			if !dispatch(len(stale)-i, func(jc context.Context) error { return env.refreshOrder(jc, o) }) {
				stopped = true
				break
			}
		}
	}
	wg.Wait()

	finished := time.Now()
	collector.Lock()
	collector.st.Running = false
	collector.st.Note = ""
	if collector.fatal != nil {
		collector.st.Note = collector.fatal.Error()
	} else if stopped {
		collector.st.Note = "to'xtatildi"
	}
	st := collector.st
	collector.Unlock()
	DB.Model(&run).Updates(map[string]any{"finished_at": &finished, "done": st.Done, "errors": st.Errors, "note": st.Note})
	log.Printf("yig'ish: yurish #%d tugadi: %d/%d, %d xato, %s %s",
		run.ID, st.Done, st.Total, st.Errors, finished.Sub(now).Round(time.Second), st.Note)
	return nil
}

// collectGap - keyingi suhbatgacha oraliq: qolgan vaqt qolgan ishga
// bo'linadi, eng kam oraliqdan tushmaydi, xatolarda backoff ko'paytiradi.
func collectGap(deadline, now time.Time, remaining int, minGap time.Duration, backoff float64) time.Duration {
	gap := minGap
	if remaining > 0 {
		if left := deadline.Sub(now); left > 0 {
			if g := left / time.Duration(remaining); g > gap {
				gap = g
			}
		}
	}
	if backoff > 1 {
		gap = time.Duration(float64(gap) * backoff)
	}
	return gap
}

// collectWait - tempo va worker chegarasini kutadi. To'xtash kerak bo'lsa false.
func collectWait(ctx context.Context, deadline time.Time, remaining int) bool {
	collector.Lock()
	backoff := collector.st.Backoff
	collector.Unlock()
	gap := collectGap(deadline, time.Now(), remaining, time.Duration(CollectMinGapMS())*time.Millisecond, backoff)
	collector.Lock()
	collector.st.GapMS = gap.Milliseconds()
	collector.Unlock()

	select {
	case <-ctx.Done():
		return false
	case <-time.After(gap):
	}
	for {
		if !collectWanted() {
			return false
		}
		collector.Lock()
		fatal, active := collector.fatal, collector.active
		collector.Unlock()
		if fatal != nil {
			return false
		}
		if active < CollectWorkers() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// collectAfterJob - natija bo'yicha hisoblagich va backoff.
func collectAfterJob(err error) {
	collector.Lock()
	defer collector.Unlock()
	collector.st.Done++
	if err == nil {
		collector.okRun++
		if collector.okRun >= collectBackoffEvery && collector.st.Backoff > 1 {
			collector.st.Backoff /= 2
			collector.okRun = 0
		}
		return
	}
	collector.st.Errors++
	collector.okRun = 0
	if collector.st.Backoff < collectBackoffMax {
		collector.st.Backoff *= 2
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrAdminkaUnauthorized) {
		// Token tugagan: qolgan hamma so'rov ham rad etiladi.
		collector.fatal = err
	}
}

// collectEnv - bir yurishdagi umumiy ulanishlar.
type collectEnv struct {
	adm   Adminka
	svc   Service
	llm   LLM
	promt *Promt
}

func newCollectEnv() *collectEnv {
	e := &collectEnv{adm: AdminkaFromEnv(), svc: ServiceFromEnv(), llm: ActiveLLM()}
	var p Promt
	if err := DB.Where("title = ?", CollectPromtTitle).First(&p).Error; err == nil {
		e.promt = &p
	} else {
		log.Printf("yig'ish: promt topilmadi, AI xulosasi yozilmaydi: %v", err)
	}
	return e
}

// lookupOrder - buyurtma holati va yetkazma tekshiruvi.
func (e *collectEnv) lookupDelivery(o AdminkaOrder) ([]DeliveryOrder, error) {
	track := trackKey(o.ExpressNum)
	if track == "" {
		return nil, nil
	}
	token, err := ServiceToken(e.svc, ServiceTokenFile)
	if err != nil {
		return nil, err
	}
	return fetchDeliveryRetry(e.svc, token, DeliveryFilter{TrackNumber: track, Size: DefaultOrdersPerCall})
}

// collectedOrder - tekshirilgan buyurtma: yozuv va turlar.
type collectedOrder struct {
	row      CollectOrder
	findings []CollectFinding
}

// buildOrder - adminka buyurtmasidan yozuv yasaydi.
func (e *collectEnv) buildOrder(o AdminkaOrder, clientID, conversationID int64, source string) (collectedOrder, error) {
	rows, derr := e.lookupDelivery(o)
	now := time.Now()
	v := ClassifyOrder(CollectInput{Order: o, Rows: rows, DeliveryOK: derr == nil}, now)
	out := collectedOrder{row: CollectOrder{
		OrderSN: o.OrderSN, UserID: o.UserID, ClientID: clientID, ConversationID: conversationID,
		Source: source, Status: o.Status, StatusLabel: StatusLabel(o.Status), PayStatus: o.PayStatus,
		ExpressNum: o.ExpressNum, OrderCreatedAt: parseOrderTime(o.CreatedAt),
		ShippedAt: parseOrderTime(o.ShippedAt), Province: o.Province,
		Verified: v.Verified, Arrived: v.Arrived, Received: v.Received, CheckedAt: now,
	}}
	if v.Row != nil {
		out.row.Delivered = v.Row.Delivered
		out.row.Branch = firstNonEmpty(v.Row.BranchName, v.Row.LocationNumber)
		out.row.ArrivedAt = parseOrderTime(v.Row.CreatedAt)
		out.row.DeliveredAt = parseOrderTime(v.Row.DeliveredAt)
	}
	for _, f := range v.Findings {
		out.findings = append(out.findings, CollectFinding{
			OrderSN: o.OrderSN, Type: f.Type, Bucket: f.Bucket, UserID: o.UserID,
			AnchorAt: f.AnchorAt, AnchorKind: f.AnchorKind,
		})
	}
	return out, derr
}

// processChat - bitta suhbatni o'rganadi va natijani darhol yozadi.
func (e *collectEnv) processChat(ctx context.Context, c Chat) error {
	start := time.Now()
	rec := CollectChat{ConversationID: c.ID, ClientID: c.ClientID, LastMessageAt: c.MsCreatedAt}
	var prev CollectChat
	if err := DB.First(&prev, "conversation_id = ?", c.ID).Error; err == nil && prev.LastMessageAt == c.MsCreatedAt {
		rec.Attempts = prev.Attempts
	}
	rec.Attempts++
	finish := func(orders []collectedOrder, jobErr error) error {
		now := time.Now()
		rec.ProcessedAt = &now
		rec.DurationMS = now.Sub(start).Milliseconds()
		if err := saveCollected(rec, orders); err != nil {
			return errors.Join(jobErr, err)
		}
		return jobErr
	}

	msgs, err := fetchHistory(c.ID)
	if err != nil {
		rec.State, rec.Error = CollectStateError, err.Error()
		return finish(nil, err)
	}
	if n := len(msgs); n > 0 {
		rec.LastMessageID = msgs[n-1].ID
	}
	rec.LastText = trimText(lastClientText(msgs), 500)
	rec.ClientWroteAt = lastClientTime(msgs)

	sns, exs := ExtractNumbers(msgs)
	textNums := mergeNumbers(sns, exs, collectMaxNumbers)
	fromText := make(map[string]bool, len(textNums))
	for _, n := range textNums {
		fromText[n] = true
	}
	numbers := textNums
	if img, ok := ReadNumbersFromMessages(ctx, msgs); ok {
		rec.ImagesRead = img.Images
		numbers = mergeNumbers(textNums, img.All(), collectMaxNumbers)
	}
	rec.Numbers = strings.Join(numbers, ",")

	var (
		orders   []collectedOrder
		notFound []string
		jobErr   error
		seen     = map[string]bool{}
	)
	for _, n := range numbers {
		found, err := findOrderByNumber(e.adm, n)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				err = ErrAdminkaUnauthorized
			}
			jobErr = errors.Join(jobErr, fmt.Errorf("%s: %w", n, err))
			continue
		}
		if len(found) == 0 {
			notFound = append(notFound, n)
			continue
		}
		source := "rasm"
		if fromText[n] {
			source = "matn"
		}
		for _, o := range dedupOrders(found) {
			if o.OrderSN == "" || seen[o.OrderSN] {
				continue
			}
			seen[o.OrderSN] = true
			co, err := e.buildOrder(o, c.ClientID, c.ID, source)
			if err != nil {
				jobErr = errors.Join(jobErr, fmt.Errorf("%s yetkazma: %w", o.OrderSN, err))
			}
			orders = append(orders, co)
		}
	}
	rec.NotFound = strings.Join(notFound, ",")

	// Mijoz raqamini aytmagan bo'lsa ham uning status 3 va 12 dagi tovarlari.
	account, err := e.collectAccountOrders(c.ClientID)
	if err != nil {
		jobErr = errors.Join(jobErr, err)
	}
	for _, o := range account {
		if o.OrderSN == "" || seen[o.OrderSN] {
			continue
		}
		seen[o.OrderSN] = true
		co, err := e.buildOrder(o, c.ClientID, c.ID, CollectSourceAccount)
		if err != nil {
			jobErr = errors.Join(jobErr, fmt.Errorf("%s yetkazma: %w", o.OrderSN, err))
		}
		orders = append(orders, co)
	}
	rec.OrdersCount = len(orders)

	e.applyAI(ctx, msgs, &rec, orders)

	switch {
	case jobErr != nil:
		rec.State, rec.Error = CollectStatePartial, jobErr.Error()
	case len(numbers) == 0 && len(orders) == 0:
		rec.State = CollectStateNoNumbers
	default:
		for _, o := range orders {
			if !o.row.Verified {
				rec.State = CollectStatePartial
			}
		}
		if rec.State == "" {
			rec.State = CollectStateDone
		}
	}
	return finish(orders, jobErr)
}

// refreshOrder - eskirgan buyurtmani qayta tekshiradi. AI xulosasi saqlanadi.
func (e *collectEnv) refreshOrder(_ context.Context, old CollectOrder) error {
	found, err := FetchOrders(e.adm, OrderFilter{OrderSN: old.OrderSN, Size: DefaultOrdersPerCall})
	if err != nil {
		if errors.Is(err, ErrUnauthorized) {
			return ErrAdminkaUnauthorized
		}
		return err
	}
	if len(found) == 0 {
		return nil
	}
	co, derr := e.buildOrder(found[0], old.ClientID, old.ConversationID, old.Source)
	co.row.AIType, co.row.AINote = old.AIType, old.AINote
	return errors.Join(derr, saveOrders(DB, []collectedOrder{co}))
}

// collectAccountStatuses - yozgan mijozning shu statusdagi buyurtmalari, u
// raqamini aytmagan bo'lsa ham olinadi.
var collectAccountStatuses = []int{StatusPaid, StatusAbnormal}

// collectAccountOrders - mijozning status 3 va 12 dagi buyurtmalari. Server
// filtrga amal qilmasa ham boshqa status yoki boshqa mijoz kirmaydi.
func (e *collectEnv) collectAccountOrders(clientID int64) ([]AdminkaOrder, error) {
	if clientID <= 0 {
		return nil, nil
	}
	var out []AdminkaOrder
	var errs error
	for _, st := range collectAccountStatuses {
		part, err := FetchOrders(e.adm, OrderFilter{UserID: clientID, Status: fmt.Sprint(st), Size: collectAccountPageSize})
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				err = ErrAdminkaUnauthorized
			}
			errs = errors.Join(errs, fmt.Errorf("mijoz %d status %d: %w", clientID, st, err))
			continue
		}
		for _, o := range part {
			if o.Status == st && o.UserID == clientID {
				out = append(out, o)
			}
		}
	}
	return out, errs
}

// lastClientText - mijozning oxirgi matnli xabari.
func lastClientText(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() && strings.TrimSpace(msgs[i].Message) != "" {
			return msgs[i].Message
		}
	}
	return ""
}

// lastClientTime - mijozning oxirgi xabari vaqti.
func lastClientTime(msgs []Message) *time.Time {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].FromClient() {
			return parseOrderTime(msgs[i].CreatedAt)
		}
	}
	return nil
}

// collectFact - modelga ketadigan buyurtma fakti.
type collectFact struct {
	OrderSN     string   `json:"order_sn"`
	Status      int      `json:"status"`
	StatusLabel string   `json:"status_label"`
	Yetkazmada  bool     `json:"yetkazmada"`
	MijozOlgan  bool     `json:"mijoz_olgan"`
	KodTurlari  []string `json:"kod_turlari"`
	Manba       string   `json:"manba"` // matn, rasm yoki akkaunt (mijoz raqamini aytmagan)
}

// collectAIResult - model javobi.
type collectAIResult struct {
	Xulosalar []struct {
		OrderSN *string `json:"order_sn"`
		Tur     string  `json:"tur"`
		Izoh    string  `json:"izoh"`
	} `json:"xulosalar"`
}

// applyAI - AI xulosasini alohida maydonlarga yozadi. Xato bo'lsa xulosa
// bo'sh qoladi, kod natijasiga ta'sir qilmaydi.
func (e *collectEnv) applyAI(ctx context.Context, msgs []Message, rec *CollectChat, orders []collectedOrder) {
	if e.promt == nil || !e.llm.Ready() || lastClientText(msgs) == "" {
		return
	}
	facts := make([]collectFact, 0, len(orders))
	for _, o := range orders {
		types := make([]string, 0, len(o.findings))
		for _, f := range o.findings {
			types = append(types, f.Type)
		}
		facts = append(facts, collectFact{o.row.OrderSN, o.row.Status, o.row.StatusLabel, o.row.Arrived, o.row.Received, types, o.row.Source})
	}
	data, _ := json.Marshal(map[string]any{"buyurtmalar": facts, "topilmagan_raqamlar": rec.NotFound})
	user := "Oxirgi xabarlar:\n" + formatTranscript(msgs) + "\n\nTizim faktlari:\n" + string(data)

	raw, u, err := e.llm.Generate(ctx, e.promt.Promt, user)
	rec.PromptTokens, rec.OutputTokens = u.PromptTokens, u.CompletionTokens
	if err != nil {
		log.Printf("yig'ish: suhbat %d AI: %v", rec.ConversationID, err)
		return
	}
	applyAIResult(raw, rec, orders)
}

// applyAIResult - model javobini yozuvlarga tarqatadi. Noto'g'ri JSON yoki
// noma'lum tur e'tiborsiz qoldiriladi.
func applyAIResult(raw string, rec *CollectChat, orders []collectedOrder) {
	var res collectAIResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &res); err != nil {
		log.Printf("yig'ish: suhbat %d AI javobi JSON emas", rec.ConversationID)
		return
	}
	for _, x := range res.Xulosalar {
		tur := strings.TrimSpace(x.Tur)
		if !collectAITypes[tur] {
			continue
		}
		note := trimText(strings.TrimSpace(x.Izoh), 500)
		matched := false
		if x.OrderSN != nil {
			for i := range orders {
				if strings.EqualFold(orders[i].row.OrderSN, strings.TrimSpace(*x.OrderSN)) {
					orders[i].row.AIType, orders[i].row.AINote = tur, note
					matched = true
				}
			}
		}
		if !matched && rec.AIType == "" {
			rec.AIType, rec.AINote = tur, note
		}
	}
	// Model buyurtma raqamini ko'rsatmay umumiy xulosa yozgan bo'lsa, u shu
	// suhbatdagi xulosasiz buyurtmalarga ham tegishli.
	for i := range orders {
		if orders[i].row.AIType == "" {
			orders[i].row.AIType, orders[i].row.AINote = rec.AIType, rec.AINote
		}
	}
}

// saveCollected - suhbat va buyurtmalarni bitta tranzaksiyada yozadi.
func saveCollected(rec CollectChat, orders []collectedOrder) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&rec).Error; err != nil {
			return err
		}
		return saveOrders(tx, orders)
	})
}

// saveOrders - buyurtma va uning turlarini almashtiradi.
func saveOrders(db *gorm.DB, orders []collectedOrder) error {
	for i := range orders {
		o := &orders[i]
		// Xodim o'chirgan buyurtma qaytib kelmaydi.
		var dismissed int64
		if err := db.Model(&CollectDismissed{}).Where("order_sn = ?", o.row.OrderSN).Count(&dismissed).Error; err != nil {
			return err
		}
		if dismissed > 0 {
			continue
		}
		if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&o.row).Error; err != nil {
			return err
		}
		// Tekshirib bo'lmagan buyurtmaning eski turlari yo'qolmasin.
		if !o.row.Verified {
			continue
		}
		if err := db.Where("order_sn = ?", o.row.OrderSN).Delete(&CollectFinding{}).Error; err != nil {
			return err
		}
		if len(o.findings) > 0 {
			if err := db.Create(&o.findings).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
