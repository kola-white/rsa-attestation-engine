"""Small private S3 operations used only by the owner-run case deletion procedure.

No SDK installation, credential arguments, redirects, content downloads or logging.
"""
import datetime
import hashlib
import hmac
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as XML


class StorageFailure(Exception):
    def __init__(self, operation, status=None):
        super().__init__('private storage operation failed')
        self.operation = operation
        self.status = status


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def signed_headers(method, url, region, key, secret, now=None):
    now = now or datetime.datetime.now(datetime.timezone.utc)
    stamp = now.strftime('%Y%m%dT%H%M%SZ')
    day = stamp[:8]
    parsed = urllib.parse.urlsplit(url)
    empty = hashlib.sha256(b'').hexdigest()
    names = 'host;x-amz-content-sha256;x-amz-date'
    canonical = (method + '\n' + parsed.path + '\n' + parsed.query + '\nhost:' + parsed.netloc
                 + '\nx-amz-content-sha256:' + empty + '\nx-amz-date:' + stamp + '\n\n' + names + '\n' + empty)
    scope = day + '/' + region + '/s3/aws4_request'
    message = 'AWS4-HMAC-SHA256\n' + stamp + '\n' + scope + '\n' + hashlib.sha256(canonical.encode()).hexdigest()
    def mac(value, text):
        return hmac.new(value, text.encode(), hashlib.sha256).digest()
    signing = mac(mac(mac(mac(('AWS4' + secret).encode(), day), region), 's3'), 'aws4_request')
    signature = hmac.new(signing, message.encode(), hashlib.sha256).hexdigest()
    return {'X-Amz-Date': stamp, 'X-Amz-Content-Sha256': empty,
            'Authorization': 'AWS4-HMAC-SHA256 Credential=' + key + '/' + scope
                             + ', SignedHeaders=' + names + ', Signature=' + signature}


class PrivateObjectStore:
    def __init__(self, settings):
        self.endpoint = settings['DO_ENDPOINT'].rstrip('/')
        self.bucket = settings['EVT_S3_BUCKET']
        self.region = settings['DO_REGION']
        self._key = settings['DO_SPACES_KEY']
        self._secret = settings['DO_SPACES_SECRET']
        parsed = urllib.parse.urlsplit(self.endpoint)
        if parsed.scheme != 'https' or parsed.username or parsed.password or parsed.query or parsed.fragment:
            raise StorageFailure('configuration')
        self._opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def request(self, method, key=None, query=None):
        url = self.endpoint + '/' + urllib.parse.quote(self.bucket, safe='')
        if key is not None:
            url += '/' + urllib.parse.quote(key, safe='/-_.~')
        if query:
            url += '?' + urllib.parse.urlencode(sorted(query.items()), quote_via=urllib.parse.quote, safe='-_.~')
        headers = signed_headers(method, url, self.region, self._key, self._secret)
        request = urllib.request.Request(url, method=method, headers=headers)
        try:
            with self._opener.open(request, timeout=30) as response:
                # GET is only ListObjects XML; HEAD/DELETE never fetch evidence.
                data = response.read(8 * 1024 * 1024 + 1) if method == 'GET' else b''
                if len(data) > 8 * 1024 * 1024:
                    raise StorageFailure(method, response.status)
                return response.status, data
        except urllib.error.HTTPError as error:
            status = error.code
            error.close()
            return status, b''
        except StorageFailure:
            raise
        except Exception:
            raise StorageFailure(method) from None

    def list_keys(self, prefix, maximum=1000):
        keys = set()
        token = None
        seen = set()
        while True:
            query = {'list-type': '2', 'prefix': prefix, 'max-keys': '1000'}
            if token:
                query['continuation-token'] = token
            status, body = self.request('GET', query=query)
            if status != 200:
                raise StorageFailure('LIST', status)
            try:
                root = XML.fromstring(body)
                if root.tag.rsplit('}', 1)[-1] != 'ListBucketResult' or root.findtext('{*}IsTruncated') not in ('true', 'false'):
                    raise StorageFailure('LIST_FORMAT')
                for row in root.findall('{*}Contents'):
                    key = row.findtext('{*}Key')
                    if not key or not key.startswith(prefix):
                        raise StorageFailure('LIST')
                    keys.add(key)
                    if len(keys) > maximum:
                        raise StorageFailure('LIST_LIMIT')
                if root.findtext('{*}IsTruncated') != 'true':
                    return keys
                token = root.findtext('{*}NextContinuationToken')
                if not token or token in seen or len(token) > 4096:
                    raise StorageFailure('LIST_PAGINATION')
                seen.add(token)
            except StorageFailure:
                raise
            except Exception:
                raise StorageFailure('LIST_FORMAT') from None

    def delete(self, key):
        status, _ = self.request('DELETE', key)
        if status != 204:
            raise StorageFailure('DELETE', status)

    def absent(self, key):
        status, _ = self.request('HEAD', key)
        if status == 404:
            return True
        if status == 200:
            return False
        raise StorageFailure('HEAD', status)
