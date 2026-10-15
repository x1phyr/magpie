// Run with Node's test runner and Playwright on the module path; see README.md.
// A model served as OpenAI's native Responses API serves it (#1571): a
// model chip's right-click, on a custom provider with a Responses URL
// (nativeCapable), has "Native Responses compatible" beside its API pick,
// and once on, "Native model: by its ID…" to name Codex's own slug for it
// in a small box. Both are staged as a model's API is — the chip tagged,
// nothing saved — and sent with the Save as modelPrefs' native and
// nativeSlug. A model asked on another API alone has it off, saying why; a
// provider that can't (a subscription's) has no such item. In English,
// Chinese, Japanese and German, at a wide and a narrow window.
const assert = require("node:assert/strict");
const fs = require("node:fs/promises");
const path = require("node:path");
const { test } = require("node:test");
const { chromium, webkit } = require("playwright");

const assets = path.resolve(__dirname, "../assets");
const base = { icon: "generic", catalog: "", agents: [], fallback: [], headers: {}, keyList: [], balanceToken: { takes: false, set: false }, proxy: "" };
const relay = {
  ...base, id: "relay", name: "Relay", chat: "https://relay.example.com/v1", responses: "https://relay.example.com/v1", anthropic: "", nativeCapable: true,
  speaks: ["chat", "responses"],
  models: [{ id: "tx-gpt-6.1-sol-standard", name: "tx-gpt-6.1-sol-standard", on: true }, { id: "gpt-6-astra", name: "gpt-6-astra", on: true, native: true },
    { id: "gpt-chat-only", name: "gpt-chat-only", on: true, api: "chat" }],
  key: { set: true, masked: "sk-…one" },
};
const sub = {
  ...base, id: "copilot", name: "GitHub Copilot", icon: "githubcopilot",
  chat: "https://api.githubcopilot.com", responses: "", anthropic: "",
  account: { agent: "copilot", user: "octo", accounts: [] },
  models: [{ id: "gpt-9", name: "GPT-9", on: true }],
  key: { set: false, masked: "" },
};

function serve(lang, posts) {
  const providers = { providers: [relay, sub], presets: [], excluded: [], gateway: { running: true, window: true } };
  const state = { agents: [], profiles: [], settings: { lang, theme: "light" } };
  return async (route) => {
    const url = new URL(route.request().url());
    const json = (data) => route.fulfill({ json: data });
    if (url.pathname === "/boot.js") return route.fulfill({ contentType: "text/javascript", body: `window.bootPrefs = {lang:"${lang}",theme:"light",web:true};` });
    if (url.pathname === "/wails/runtime.js") return route.fulfill({ contentType: "text/javascript", body: "export const Window = {};" });
    if (url.pathname === "/api/state") return json(state);
    if (url.pathname === "/api/providers") return json(providers);
    if (url.pathname === "/api/groups") return json({ groups: [] });
    if (url.pathname === "/api/provider/save") { posts.push(route.request().postDataJSON()); return json(providers); }
    if (url.pathname.startsWith("/api/")) return json({});
    const file = path.join(assets, url.pathname === "/" ? "index.html" : url.pathname);
    const contentType = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png" }[path.extname(file)];
    await route.fulfill({ body: await fs.readFile(file), contentType });
  };
}

const words = {
  en: { bad: "Not an OpenAI model's slug: as gpt-6.1-sol or o4-mini", on: "Native Responses compatible", slug: "Native model: by its ID…", slugSet: "Native model: gpt-6.1-sol…", tag: "native", set: "Set", save: "Save", why: "Asked on OpenAI: only a model asked on Responses can be" },
  zh: { bad: "不是 OpenAI 模型的 slug：如 gpt-6.1-sol 或 o4-mini", on: "原生 Responses 兼容", slug: "原生模型：按模型 ID…", slugSet: "原生模型：gpt-6.1-sol…", tag: "原生", set: "设置", save: "保存", why: "走 OpenAI：只有走 Responses 的模型才能开启" },
  ja: { bad: "OpenAI モデルの slug ではありません：例 gpt-6.1-sol、o4-mini", on: "ネイティブ Responses 互換", slug: "ネイティブモデル：ID で対応…", slugSet: "ネイティブモデル：gpt-6.1-sol…", tag: "ネイティブ", set: "設定", save: "保存", why: "OpenAI で送信：Responses で送るモデルだけが対象です" },
  de: { bad: "Kein Slug eines OpenAI-Modells: etwa gpt-6.1-sol oder o4-mini", on: "Native-Responses-kompatibel", slug: "Natives Modell: nach ID…", slugSet: "Natives Modell: gpt-6.1-sol…", tag: "nativ", set: "Setzen", save: "Speichern", why: "Über OpenAI: nur ein Modell über Responses kann es sein" },
};

async function rclick(loc) {
  await loc.click({ button: "right", trial: true });
  await loc.page().evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  await loc.click({ button: "right" });
}

for (const engine of (process.env.BROWSER ? [process.env.BROWSER] : ["chromium", "webkit"])) {
  for (const lang of Object.keys(words)) {
    for (const width of [900, 420]) {
      const w = words[lang];
      test(`${engine} ${lang} ${width}: a model set native Responses, its slug named, staged and saved`, async (t) => {
        const browser = await (engine === "webkit" ? webkit.launch() : chromium.launch({ channel: "chromium" }));
        const page = await (await browser.newContext({ viewport: { width, height: 760 }, reducedMotion: "reduce" })).newPage();
        t.after(async () => {
          if (process.env.ARTIFACT_DIR) {
            await fs.mkdir(process.env.ARTIFACT_DIR, { recursive: true });
            await page.screenshot({ path: path.join(process.env.ARTIFACT_DIR, `${engine}-${lang}-${width}-native-responses.png`) });
          }
          await browser.close();
        });
        page.setDefaultTimeout(5000);
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        const posts = [];
        await page.route("**/*", serve(lang, posts));
        await page.goto("http://magpie.test/?view=providers");
        const open = async (name) => {
          await page.locator(".row.provider", { hasText: name }).first().click();
          await page.locator(".editor .mchips .mchip").first().waitFor();
        };
        const chip = (id) => page.locator(".editor .mchips .mchip", { hasText: id });
        const rowMenu = page.locator(".pop.row-menu");
        const item = (name) => rowMenu.getByRole("menuitem", { name });

        await open("Relay");
        // one set before: its tag, and the slug item
        assert.equal(await chip("gpt-6-astra").locator(".mnative-tag").textContent(), w.tag);
        await rclick(chip("gpt-6-astra"));
        await item(w.slug).waitFor();
        await page.keyboard.press("Escape");
        await rowMenu.waitFor({ state: "detached" });

        // asked on another API alone: off, saying why
        await rclick(chip("gpt-chat-only"));
        assert(await item(w.on).isDisabled(), "off on a chat-only model");
        assert((await item(w.on).textContent()).includes(w.why));
        await page.keyboard.press("Escape");
        await rowMenu.waitFor({ state: "detached" });

        const tx = chip("tx-gpt-6.1-sol-standard");
        assert.equal(await tx.locator(".mnative-tag").count(), 0, "off by default");
        await rclick(tx);
        assert.equal(await item(w.slug).count(), 0, "no slug while off");
        await item(w.on).click();
        await chip("tx-gpt-6.1-sol-standard").locator(".mnative-tag").waitFor();
        await rclick(chip("tx-gpt-6.1-sol-standard"));
        await item(w.slug).click();
        const box = page.locator(".pop.native-slug-menu");
        await box.waitFor();
        const bb = await box.boundingBox();
        assert(bb.x >= 0 && bb.x + bb.width <= width + 0.5, `the box fits the window: ${JSON.stringify(bb)}`);
        // what the backend turns away can't be set: Set off, the hint says why
        const setBtn = box.getByRole("button", { name: w.set });
        await box.locator("input.native-slug").fill("claude-x");
        assert(await setBtn.isDisabled(), "Set off for a non-OpenAI slug");
        assert.equal(await box.locator("input.native-slug").getAttribute("aria-invalid"), "true");
        assert.equal(await box.locator(".hint").textContent(), w.bad);
        await box.locator("input.native-slug").press("Enter");
        assert(await box.isVisible(), "Enter doesn't take it either");
        await box.locator("input.native-slug").fill("");
        assert(!(await setBtn.isDisabled()), "empty is by its ID");
        await box.locator("input.native-slug").fill("GPT-6.1-sol");
        await box.getByRole("button", { name: w.set }).click();
        await box.waitFor({ state: "detached" });
        await rclick(chip("tx-gpt-6.1-sol-standard"));
        await item(w.slugSet).waitFor();
        await page.keyboard.press("Escape");
        assert.equal(posts.length, 0, "staged, not saved");
        await page.locator(".editor").getByRole("button", { name: w.save, exact: true }).click();
        for (let i = 0; i < 40 && !posts.length; i++) await page.waitForTimeout(50);
        assert.deepEqual(posts[0]?.modelPrefs, { "tx-gpt-6.1-sol-standard": { native: true, nativeSlug: "gpt-6.1-sol" } });

        await page.goto("http://magpie.test/?view=providers");
        await open("GitHub Copilot");
        await rclick(chip("GPT-9"));
        await rowMenu.waitFor();
        assert.equal(await item(w.on).count(), 0, "a subscription can't");
        assert.deepEqual(errors, []);
      });
    }
  }
}
