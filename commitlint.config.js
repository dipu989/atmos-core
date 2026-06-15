module.exports = {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [
      2,
      'always',
      ['feat', 'fix', 'perf', 'revert', 'chore', 'docs', 'refactor', 'test', 'ci'],
    ],
    'subject-case': [0],
  },
}
