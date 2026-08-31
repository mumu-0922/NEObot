import { expect, test } from "@playwright/test";

import {
  NeoChatApiFixture,
  TEST_RECOVERY_TOKEN,
  conversation,
} from "./fixtures/neoChatApi";

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

test("password recovery resets the credential and returns to login", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(
      "20000000-0000-4000-8000-000000000102",
      "Recovered chat",
      "gpt-5.6-luna",
    ),
  ]);
  await api.install(page);

  await page.goto("/");
  await page.getByRole("button", { name: "忘记密码？" }).click();
  await page.getByLabel("邮箱").fill("owner@example.test");
  await page.getByRole("button", { name: "发送找回邮件" }).click();

  await page.getByLabel("Recovery Token").fill(TEST_RECOVERY_TOKEN);
  await page.getByLabel("新密码", { exact: true }).fill("Recovered+Pass1");
  await page.getByLabel("确认新密码").fill("Recovered+Pass1");
  await page.getByRole("button", { name: "重置密码" }).click();

  await expect(
    page.getByText("密码修改成功。", { exact: false }),
  ).toBeVisible();
  await page.getByRole("button", { name: "返回登录" }).click();
  await page.getByLabel("邮箱").fill("owner@example.test");
  await page.getByLabel("密码").fill("Recovered+Pass1");
  await page.getByLabel("密码").press("Enter");

  await expect(
    page.getByRole("button", { name: "Recovered chat", exact: true }),
  ).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});

test("changing a password rejects whitespace, accepts ASCII symbols, and revokes the current session", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([
    conversation(
      "20000000-0000-4000-8000-000000000103",
      "Secured chat",
      "gpt-5.6-luna",
    ),
  ]);
  await api.install(page);
  await api.authenticate(page);

  await page.goto("/?panel=settings&settingsTab=account");
  await page.getByLabel("当前密码").fill("e2e-pass");
  await page.getByLabel("新密码", { exact: true }).fill("Secure Pass1!");
  await page.getByLabel("确认新密码").fill("Secure Pass1!");
  await page.getByRole("button", { name: "修改密码" }).click();

  await expect(
    page.getByText("新密码只能包含英文字母、数字和符号", { exact: false }),
  ).toBeVisible();

  await page.getByLabel("新密码", { exact: true }).fill("Secure+Pass1!");
  await page.getByLabel("确认新密码").fill("Secure+Pass1!");
  await page.getByRole("button", { name: "修改密码" }).click();

  await expect(page.getByLabel("邮箱")).toBeVisible();
  await page.getByLabel("邮箱").fill("owner@example.test");
  await page.getByLabel("密码").fill("Secure+Pass1!");
  await page.getByLabel("密码").press("Enter");

  await expect(
    page.getByRole("button", { name: "Secured chat", exact: true }),
  ).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});

test("revoke all sessions returns the current browser to login", async ({
  page,
}) => {
  const api = new NeoChatApiFixture([]);
  await api.install(page);
  await api.authenticate(page);

  await page.goto("/?panel=settings&settingsTab=account");
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "退出所有设备" }).click();

  await expect(page.getByLabel("邮箱")).toBeVisible();
  await expect(page.getByLabel("密码")).toBeVisible();
  expect(api.unhandledRequests).toEqual([]);
});
