# SLOTH-GO
<h2>Same as the original, but written in GO</h2>

Enter values for the following in your config.json:<br />
<strong>Input Path:</strong> The path of the files you want to move<br />
<strong>Output Path:</strong> The path of where you want the files to move to<br />
<strong>Pattern:</strong> The file extension to search for (csv, docx, txt, xls, zip, etc (all extensions supported))<br />
<strong>Folder Type:</strong> What output file structure to create in your output path<br />
<i>Example json below</i><br />
<code>
[<br />
        {<br />
          "name": "Files By Type - Zip",<br />
          "input": "C:\\somepath\\processed",<br />
          "output": "C:\\somepath\\archive",<br />
          "pattern": "zip",<br />
          "folderType": "4"<br />
        },<br />
        {<br />
          "name": "Files By Type - pdf",<br />
          "input": "C:\\somepath\\processed",<br />
          "output": "C:\\somepath\\archive",<br />
          "pattern": "pdf",<br />
          "folderType": "4"<br />
        },<br />
        {<br />
          "name": "Pictures - jpg",<br />
          "input": "C:\\somepath\\processed",<br />
          "output": "C:\\somepath\\archive",<br />
          "pattern": "jpg",<br />
          "folderType": "4"<br />
        }<br />
]<br />
      </code>
<ul>
<li>By modified date (moddate):  1</li>
<li>By file extension (pattern):  2</li>
<li>By pattern then year:  3</li>
<li>Simple move from folder to folder (none):  4</li>
<li>YYYYMM as folder:  5</li>
</ul>