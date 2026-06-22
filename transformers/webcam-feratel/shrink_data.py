import xml.etree.ElementTree as ET
import json

# Parse the large raw.xml
tree = ET.parse('testdata/raw.xml')
root = tree.getroot()

# Find the links element
links = root.find('.//links')
if links is not None:
    # Keep only the first link
    first_link = links[0]
    links.clear()
    links.append(first_link)
    links.set('count', '1')

# Write the truncated XML to testdata/raw_small.xml
xml_str = ET.tostring(root, encoding='utf-8', xml_declaration=True).decode('utf-8')
with open('testdata/raw.xml', 'w') as f:
    f.write(xml_str)

# Write to in.json as a JSON string
with open('testdata/in.json', 'w') as f:
    json.dump(xml_str, f)

print("Shrunk raw.xml and generated in.json")
