import { expect, test } from "@playwright/test";

import { NeoChatApiFixture, conversation } from "./fixtures/neoChatApi";

test("login survives refresh and an expired server session returns to login", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(
      "20000000-0000-4000-8000-000000000101",
      "Authenticated chat",
      "gpt-5.6-luna",
    ),
  ]);
  await api.install(page);

  await page.goto("/");
  await expect(page.getByLabel("邮箱")).toBeVisible();

  await page.getByLabel("邮箱").fill("owner@example.test");
  await page.getByLabel("密码").fill("e2e-pass");
  await page.getByLabel("密码").press("Enter");

  await expect(
    page.getByRole("button", { name: "Authenticated chat", exact: true }),
  ).toBeVisible();

  await page.reload();
  await expect(
    page.getByRole("button", { name: "Authenticated chat", exact: true }),
  ).toBeVisible();
  await expect(page.getByLabel("邮箱")).toHaveCount(0);

  api.expireSession();
  await page.reload();
  await expect(page.getByLabel("邮箱")).toBeVisible();
  await expect(page.getByLabel("密码")).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});
