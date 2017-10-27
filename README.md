# SLOTH-GO
<h2>Same as the original, but written in GO</h2>

Enter values for the following in your config.json:<br />
<strong>Input Path:</strong> The path of the files you want to move<br />
<strong>Output Path:</strong> The path of where you want the files to move to<br />
<strong>Pattern:</strong> The file extension to search for (csv, docx, txt, xls, zip, etc (all extensions supported))<br />
<strong>Folder Type:</strong> What output file structure to create in your output path<br />
<strong>Example json below</strong><br />
<code>
[
        {
          "name": "Files By Type - Zip",
          "input": "C:\\Users\\Richardbi\\Downloads",
          "output": "C:\\Users\\Richardbi\\Documents\\Files By Type\\zip",
          "pattern": "zip",
          "folderType": "4"
        },
        {
          "name": "Files By Type - pdf",
          "input": "C:\\Users\\Richardbi\\Downloads",
          "output": "C:\\Users\\Richardbi\\Documents\\Files By Type\\pdf",
          "pattern": "pdf",
          "folderType": "4"
        },
        {
          "name": "Pictures - jpg",
          "input": "C:\\Users\\Richardbi\\Downloads",
          "output": "C:\\Users\\Richardbi\\Pictures\\jpg",
          "pattern": "jpg",
          "folderType": "4"
        },
        {
          "name": "EFS Files",
          "input": "G:\\Pubfiles\\Ops Support\\Fuel\\EFSFTPData\\Processed",
          "output": "G:\\Pubfiles\\Ops Support\\Fuel\\EFSFTPData\\Archive",
          "pattern": "txt",
          "folderType": "1"
        },
        {
          "name": "Fourkites Files",
          "input": "\\\\CRE-EXTOL\\EBICustom\\Ftp\\FourKites\\Out\\Archive",
          "output": "\\\\CRE-EXTOL\\EBICustom\\Ftp\\FourKites\\Out\\Archive",
          "pattern": "csv",
          "folderType": "1"
        }
      ]
      </code>
<ul>
<li>By modified date (moddate):  1</li>
<li>By file extension (pattern):  2</li>
<li>By pattern then year:  3</li>
<li>Simple move from folder to folder (none):  4</li>
<li>YYYYMM as folder:  5</li>
</ul>
  
Example args:<br />
<i>-inPath="C:\Users\userfolder\Downloads" -outPath="C:\Users\userfolder\Pictures" -pattern="jpg" -ftype="2"</i><br />
This will create a "jpg" folder in Pictures. Example: C:\Users\userfolder\Pictures\jpg<br />
  Then it will move all txt files from downloads to downloads\archive\txt<br />
