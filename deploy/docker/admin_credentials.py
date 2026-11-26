import secrets
import string
import sys


def admin_username():
    while True:
        value = secrets.choice(string.ascii_letters) + ''.join(
            secrets.choice(string.ascii_letters + string.digits) for _ in range(9)
        )
        if not value.lower().startswith('vr') and all(
            any(char in group for char in value)
            for group in (string.ascii_lowercase, string.ascii_uppercase, string.digits)
        ):
            return value


def admin_password(username=''):
    groups = (string.ascii_lowercase, string.ascii_uppercase, string.digits, '!%+_-')
    alphabet = ''.join(groups)
    while True:
        value = ''.join(secrets.choice(alphabet) for _ in range(18))
        if not value.lower().startswith('vr') and (
            len(username) < 3 or username.lower() not in value.lower()
        ) and all(any(char in group for char in value) for group in groups):
            return value


if __name__ == '__main__':
    if sys.argv[1:] == ['username']:
        print(admin_username())
    elif len(sys.argv) in (2, 3) and sys.argv[1] == 'password':
        print(admin_password(*sys.argv[2:]))
    else:
        raise SystemExit('Usage: admin_credentials.py username | password [USERNAME]')
