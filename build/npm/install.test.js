'use strict';

const path = require('path');
const test = require('node:test');
const assert = require('node:assert/strict');

const { assetName, binPath, welcomeText } = require('./install.js');

test('selects Windows release assets for both supported architectures', () => {
  assert.equal(assetName('win32', 'x64'), 'jenkins-cli-windows-amd64.exe');
  assert.equal(assetName('win32', 'arm64'), 'jenkins-cli-windows-arm64.exe');
});

test('uses an exe launcher target on Windows', () => {
  assert.equal(path.basename(binPath('win32').file), 'jenkins-cli.exe');
});

test('rejects unsupported Windows architectures', () => {
  assert.throws(() => assetName('win32', 'ia32'), /unsupported platform win32\/ia32/);
});

test('welcome text recommends Jenkins commands', () => {
  assert.match(welcomeText(), /jenkins-cli job list/);
  assert.match(welcomeText(), /jenkins-cli build list/);
  assert.doesNotMatch(welcomeText(), /jenkins-cli (stream|search)/);
});
