import xml.etree.ElementTree as ET
import json
import urllib.request
import os

url = "http://wtvxmlp.feratel.com/xmlpan/x3/infoxml.jsp?pg=CDB9645D-E67B-44D2-9FC9-E1539FF9A6B7&lg=de&showKeywords=1&geoXY=1&xmlv3=1&nolg=1"
try:
    with urllib.request.urlopen(url) as response:
        xml_str = response.read().decode('utf-8')
except Exception as e:
    print(f"Failed to fetch XML: {e}")
    exit(1)

with open('testdata/raw_full.xml', 'w') as f:
    f.write(xml_str)

with open('testdata/in_full.json', 'w') as f:
    json.dump(xml_str, f)

print("Generated raw_full.xml and in_full.json")
