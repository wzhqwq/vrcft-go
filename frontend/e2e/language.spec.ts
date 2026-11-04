import {expect, test} from '@playwright/test'
import {installWailsMocks} from './fixtures'

test('switches and persists interface language without saving tracking settings', async ({page}) => {
  await installWailsMocks(page)
  await page.goto('/')
  await page.getByRole('button', {name: '设置', exact: true}).click()
  await page.getByRole('button', {name: '语言', exact: true}).click()
  await page.getByRole('option', {name: 'English'}).click()
  await expect(page.getByRole('heading', {name: 'Settings', exact: true})).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en-US')
  await expect(page.getByRole('region', {name: 'Unsaved changes'})).toBeHidden()

  await page.reload()
  await expect(page.getByRole('button', {name: 'Overview', exact: true})).toBeVisible()
  await page.getByRole('button', {name: 'Settings', exact: true}).click()
  await page.getByRole('button', {name: 'Language', exact: true}).click()
  await page.getByRole('option', {name: 'Follow system'}).click()
  await expect(page.getByRole('heading', {name: '设置', exact: true})).toBeVisible()
  await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
})

test('uses English when the system language is unsupported', async ({browser}) => {
  const context = await browser.newContext({locale: 'ja-JP'})
  try {
    const page = await context.newPage()
    await installWailsMocks(page)
    await page.goto('/')
    await expect(page.getByRole('button', {name: 'Overview', exact: true})).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en-US')
  } finally {
    await context.close()
  }
})
