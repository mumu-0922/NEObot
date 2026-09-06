import { expect, test } from "@playwright/test";
import { NeoChatApiFixture } from "./fixtures/neoChatApi";
import { providerConfigs } from "./fixtures/neoChatApiSupport";

test("discovery enables an omitted model and preserves it through refresh and reload", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([]);
  await api.authenticate(page);
  await api.install(page);
  await page.route("https://basellm.github.io/**", (route) =>
    route.fulfill({ json: {} }),
  );
  const stored = providerConfigs()[0];
  let discoveries = 0;
  await page.route("**/mm-api/v1/admin/providers**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/providers") && request.method() === "GET") {
      await route.fulfill({ json: { providers: [stored] } });
    } else if (path.endsWith("/SUB/discover")) {
      discoveries++;
      await route.fulfill({
        json: {
          provider: stored,
          models:
            discoveries === 1
              ? ["gpt-5.6-luna", "gpt-6-astra"]
              : ["gpt-5.6-luna"],
          discoveredModels: discoveries === 1 ? ["gpt-6-astra"] : [],
        },
      });
    } else if (path.endsWith("/SUB") && request.method() === "PUT") {
      Object.assign(stored, request.postDataJSON());
      await route.fulfill({ json: stored });
    } else {
      await route.fallback();
    }
  });
  await page.goto("/?panel=settings&settingsTab=providers");
  const fetchModels = page.getByRole("button", {
    name: "从 Sub 获取模型",
    exact: true,
  });
  await expect(fetchModels).toBeVisible();
  await expect(page.getByText(/最多测试 3 个/)).toBeVisible();
  await fetchModels.click();
  const astra = page.getByRole("checkbox", {
    name: /gpt.*6.*astra/i,
  });
  await expect(astra).toBeChecked();
  await expect.poll(() => stored.models.includes("gpt-6-astra")).toBe(true);
  await fetchModels.click();
  await expect.poll(() => discoveries).toBe(2);
  await expect(astra).toBeChecked();
  await page.reload();
  await expect(astra).toBeChecked();
  expect(stored.models).toContain("gpt-5.6-terra");
  expect(api.unhandledRequests).toEqual([]);
});
